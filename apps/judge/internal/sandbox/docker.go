package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	compileTimeout = 30 * time.Second
	containerGrace = 3 * time.Second
	runMemoryFloor = 16

	defaultOutputLimit = 1 << 20 // 1 MiB default when no output limit is requested
	outputOverhead     = 4 << 20 // /out headroom for stderr and measurement files
	minOutTmpfs        = 8 << 20 // floor so submission/checker runs keep ample room
)

var cpuFallbackOnce sync.Once

type Docker struct {
	bin      string
	images   map[string]string
	instance string
}

func NewDocker(bin string, images map[string]string) *Docker {
	instance := os.Getenv("KAPOJ_JUDGE_INSTANCE")
	if instance == "" {
		instance, _ = os.Hostname()
	}
	if instance == "" {
		instance = "judge-" + randomHex(4)
	}
	return &Docker{bin: bin, images: images, instance: instance}
}

// Sweep removes containers left by this judge instance, for example after an
// interrupted shutdown. It never touches another instance's containers.
func (d *Docker) Sweep(ctx context.Context) {
	out, err := exec.CommandContext(ctx, d.bin, "ps", "-aq",
		"--filter", "label=kapoj.judge.instance="+d.instance).Output()
	if err != nil {
		slog.Warn("list leftover judge containers failed", "err", err)
		return
	}
	for _, id := range strings.Fields(string(out)) {
		if err := exec.CommandContext(ctx, d.bin, "rm", "-f", id).Run(); err != nil {
			slog.Warn("remove leftover judge container failed", "container", id, "err", err)
		}
	}
}

func (d *Docker) Cleanup(artifactDir string) {
	if artifactDir != "" {
		_ = os.RemoveAll(artifactDir)
	}
}

func (d *Docker) Compile(ctx context.Context, req CompileRequest) (CompileResult, error) {
	language := req.Language
	if req.Checker {
		language = "cpp"
	}
	spec, err := languageFor(language, req.Checker, 256)
	if err != nil {
		return CompileResult{}, err
	}
	image, err := d.image(language)
	if err != nil {
		return CompileResult{}, err
	}
	compileCtx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()

	name := containerName()
	if err := d.start(compileCtx, name, image, d.compileFlags()); err != nil {
		d.remove(name)
		return CompileResult{}, err
	}
	defer d.remove(name)

	if err := d.writeFile(compileCtx, name, spec.sourcePath, req.Source, 0o644); err != nil {
		return CompileResult{}, err
	}
	var out, errOut limitedBuffer
	out.max, errOut.max = 64<<10, 64<<10
	err = d.exec(compileCtx, name, nil, &out, &errOut, spec.compile...)
	if ctx.Err() != nil {
		return CompileResult{}, ctx.Err()
	}
	if errors.Is(compileCtx.Err(), context.DeadlineExceeded) {
		return CompileResult{}, errors.New("compile timed out")
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 125 || exitErr.ExitCode() == 126 || exitErr.ExitCode() == 127) {
			return CompileResult{}, fmt.Errorf("compiler container failed: %w: %s", err, errOut.String())
		}
		message := strings.TrimSpace(out.String() + "\n" + errOut.String())
		return CompileResult{Failed: true, ErrorOutput: message}, nil
	}

	artifactDir, err := os.MkdirTemp("", "kapoj-artifact-*")
	if err != nil {
		return CompileResult{}, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(artifactDir)
		}
	}()

	switch spec.artifact {
	case artifactBinary:
		dest := filepath.Join(artifactDir, "main")
		if req.Checker {
			dest = filepath.Join(artifactDir, "checker")
		}
		if err := d.copyOutFile(compileCtx, name, spec.artifactSrc, dest); err != nil {
			return CompileResult{}, err
		}
	case artifactClasses:
		if err := d.copyOutDir(compileCtx, name, spec.artifactSrc, filepath.Join(artifactDir, "classes")); err != nil {
			return CompileResult{}, err
		}
	case artifactSource:
		if err := os.WriteFile(filepath.Join(artifactDir, "main.py"), req.Source, 0o644); err != nil {
			return CompileResult{}, err
		}
	}
	ok = true
	return CompileResult{ArtifactDir: artifactDir}, nil
}

func (d *Docker) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	language := req.Language
	checker := req.Checker != nil
	if checker {
		language = "cpp"
	}
	spec, err := languageFor(language, checker, req.MemoryLimitMB)
	if err != nil {
		return RunResult{}, err
	}
	image, err := d.image(language)
	if err != nil {
		return RunResult{}, err
	}
	limit := effectiveOutputLimit(req.OutputLimitBytes)
	name := containerName()
	if err := d.start(ctx, name, image, d.runFlags(req.MemoryLimitMB, limit)); err != nil {
		d.remove(name)
		return RunResult{}, err
	}
	defer d.remove(name)

	if err := d.copyArtifacts(ctx, name, req.ArtifactDir, spec, checker); err != nil {
		return RunResult{}, err
	}
	if checker {
		if err := d.writeFileFrom(ctx, name, "/work/output.txt", req.Checker.OutputPath); err != nil {
			return RunResult{}, err
		}
		if err := d.writeFileFrom(ctx, name, "/work/answer.txt", req.Checker.AnswerPath); err != nil {
			return RunResult{}, err
		}
	}
	timeLimit := normalizeTimeLimit(req.TimeLimitMS)
	if err := d.writeRunFiles(ctx, name, spec, timeLimit, limit); err != nil {
		return RunResult{}, err
	}
	return d.runRound(ctx, name, req.InputPath, timeLimit, limit, true)
}

// RunBatch executes all cases in one run container. The container, compiled
// artifacts and wrapper script are created once; each case only replaces the
// input, runs the wrapper and reads fresh measurement files. OnResult may stop
// the batch early without affecting the remaining container lifecycle.
func (d *Docker) RunBatch(ctx context.Context, req RunBatchRequest) ([]RunResult, error) {
	if len(req.Cases) == 0 {
		return nil, nil
	}
	spec, err := languageFor(req.Language, false, req.MemoryLimitMB)
	if err != nil {
		return nil, err
	}
	image, err := d.image(req.Language)
	if err != nil {
		return nil, err
	}
	limit := effectiveOutputLimit(req.OutputLimitBytes)
	name := containerName()
	if err := d.start(ctx, name, image, d.runFlags(req.MemoryLimitMB, limit)); err != nil {
		d.remove(name)
		return nil, err
	}
	defer d.remove(name)

	if err := d.copyArtifacts(ctx, name, req.ArtifactDir, spec, false); err != nil {
		return nil, err
	}
	timeLimit := normalizeTimeLimit(req.TimeLimitMS)
	if err := d.writeRunFiles(ctx, name, spec, timeLimit, limit); err != nil {
		return nil, err
	}

	results := make([]RunResult, 0, len(req.Cases))
	for i, c := range req.Cases {
		run, err := d.runRound(ctx, name, c.InputPath, timeLimit, limit, false)
		if err != nil {
			return results, err
		}
		results = append(results, run)
		if req.OnResult != nil && !req.OnResult(i, run) {
			break
		}
	}
	return results, nil
}

func normalizeTimeLimit(ms int) int {
	if ms <= 0 {
		return 1000
	}
	return ms
}

func (d *Docker) writeRunFiles(ctx context.Context, name string, spec languageSpec, timeLimit, outputLimit int) error {
	if err := d.writeFile(ctx, name, "/work/time_limit_ms", []byte(strconv.Itoa(timeLimit)), 0o644); err != nil {
		return err
	}
	return d.writeFile(ctx, name, "/work/run.sh", []byte(runScript(spec.run, outputLimit)), 0o644)
}

func (d *Docker) runRound(ctx context.Context, name, inputPath string, timeLimit, outputLimit int, deadlineAsTLE bool) (RunResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeLimit)*time.Millisecond+containerGrace)
	defer cancel()

	var out, errOut bytes.Buffer
	run := func() error {
		input, err := os.Open(inputPath)
		if err != nil {
			return err
		}
		defer func() { _ = input.Close() }()
		out.Reset()
		errOut.Reset()
		return d.exec(runCtx, name, input, &out, &errOut, "sh", "-c",
			"cat > /work/input.txt && sh /work/run.sh")
	}
	err := run()
	if err != nil && ctx.Err() == nil && runCtx.Err() == nil {
		if exitedWithCode(err, 137) {
			if recovered, ok := d.recoverOOM(ctx, name); ok {
				return recovered, nil
			}
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = d.exec(cleanupCtx, name, nil, io.Discard, io.Discard, "sh", "-c", roundCleanupScript())
		cleanupCancel()
		err = run()
	}
	if err != nil && exitedWithCode(err, 137) {
		if recovered, ok := d.recoverOOM(ctx, name); ok {
			return recovered, nil
		}
	}
	if ctx.Err() != nil {
		return RunResult{}, ctx.Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		if deadlineAsTLE {
			return RunResult{TimedOut: true, ExitCode: -1, TimeMS: timeLimit + 1}, nil
		}
		return RunResult{}, fmt.Errorf("run container timed out")
	}
	if err != nil {
		return RunResult{}, fmt.Errorf("run container: %w: %s", err, errOut.String())
	}

	files, err := d.readRoundFiles(ctx, name)
	if err != nil {
		return RunResult{}, err
	}
	exitCode, err := measurementInt(files, "exit_code")
	if err != nil {
		return RunResult{}, err
	}
	timedOut, err := measurementInt(files, "timed_out")
	if err != nil {
		return RunResult{}, err
	}
	timeMS, err := measurementInt(files, "time_ms")
	if err != nil {
		return RunResult{}, err
	}
	cpuMS, cpuErr := measurementInt(files, "cpu_ms")
	if cpuErr != nil || cpuMS < 0 {
		cpuFallbackOnce.Do(func() {
			slog.Warn("cgroup CPU time unavailable; falling back to wall clock")
		})
		cpuMS = timeMS
	}
	memoryKB, err := measurementInt(files, "memory_kb")
	if err != nil {
		return RunResult{}, err
	}
	oomKills, err := measurementInt(files, "oom_kill")
	if err != nil {
		return RunResult{}, err
	}
	output := files["stdout"]
	truncated := len(output) > outputLimit
	if truncated {
		output = output[:outputLimit]
	}
	stderr := files["stderr"]
	if len(stderr) > 4096 {
		stderr = stderr[:4096]
	}
	return RunResult{
		Output: output, Stderr: stderr, ExitCode: exitCode,
		TimedOut:  timedOut != 0,
		OOMKilled: oomKills > 0 || exitCode == 137,
		TimeMS:    timeMS, CPUTimeMS: cpuMS, MemoryKB: memoryKB, OutputTruncated: truncated,
	}, nil
}

// readRoundFiles transfers /out once per round instead of issuing one docker
// exec per measurement file. /out is root-owned and bounded by its tmpfs.
func (d *Docker) readRoundFiles(ctx context.Context, name string) (map[string][]byte, error) {
	var buf bytes.Buffer
	var errOut limitedBuffer
	errOut.max = 4096
	if err := d.exec(ctx, name, nil, &buf, &errOut, "tar", "cf", "-", "-C", "/out", "."); err != nil {
		return nil, fmt.Errorf("read round output: %w: %s", err, errOut.String())
	}
	files := make(map[string][]byte)
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read round output tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
			continue
		}
		name := strings.TrimPrefix(filepath.Clean(hdr.Name), "./")
		data, err := io.ReadAll(io.LimitReader(tr, 64<<20))
		if err != nil {
			return nil, fmt.Errorf("read round output %s: %w", name, err)
		}
		files[name] = data
	}
	return files, nil
}

func exitedWithCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

// recoverOOM handles the Linux cgroup v2 case where the OOM killer also kills
// the root wrapper shell. The wrapper has already written the pre-run oom_kill
// counter, so a higher current value proves this was an OOM, not an exec error.
func (d *Docker) recoverOOM(ctx context.Context, name string) (RunResult, bool) {
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var buf bytes.Buffer
	err := d.exec(readCtx, name, nil, &buf, io.Discard, "sh", "-c", `
before=0
[ -r /out/oom_before ] && before=$(cat /out/oom_before)
after=0
if [ -r /sys/fs/cgroup/memory.events ]; then
  while read -r key value; do
    [ "$key" = "oom_kill" ] && after=$value
  done < /sys/fs/cgroup/memory.events
fi
echo "$before $after"
`)
	if err != nil {
		return RunResult{}, false
	}
	fields := strings.Fields(buf.String())
	if len(fields) != 2 {
		return RunResult{}, false
	}
	before, beforeErr := strconv.Atoi(fields[0])
	after, afterErr := strconv.Atoi(fields[1])
	if beforeErr != nil || afterErr != nil || after <= before {
		return RunResult{}, false
	}
	return RunResult{ExitCode: 137, OOMKilled: true}, true
}

func measurementInt(files map[string][]byte, name string) (int, error) {
	data, ok := files[name]
	if !ok {
		return 0, fmt.Errorf("measurement %s is missing", name)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, fmt.Errorf("measurement %s is missing", name)
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("measurement %s is invalid: %q", name, text)
	}
	return n, nil
}

func (d *Docker) image(language string) (string, error) {
	image := d.images[language]
	if image == "" {
		return "", fmt.Errorf("no Docker image configured for language %q", language)
	}
	return image, nil
}

func (d *Docker) start(ctx context.Context, name, image string, flags []string) error {
	args := append([]string{"run", "-d", "--name", name}, flags...)
	args = append(args, image, "sleep", "3600")
	out, err := exec.CommandContext(ctx, d.bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run: %w: %s", err, truncate(string(out), 2048))
	}
	return nil
}

func (d *Docker) remove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, d.bin, "rm", "-f", name).Run()
}

func (d *Docker) exec(ctx context.Context, name string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	dockerArgs := []string{"exec"}
	if stdin != nil {
		dockerArgs = append(dockerArgs, "-i")
	}
	dockerArgs = append(dockerArgs, name)
	dockerArgs = append(dockerArgs, args...)
	cmd := exec.CommandContext(ctx, d.bin, dockerArgs...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (d *Docker) writeFile(ctx context.Context, name, dest string, data []byte, mode os.FileMode) error {
	cmd := "cat > " + dest
	if mode&0o111 != 0 {
		cmd += " && chmod 755 " + dest
	}
	var errOut limitedBuffer
	errOut.max = 4096
	if err := d.exec(ctx, name, bytes.NewReader(data), io.Discard, &errOut, "sh", "-c", cmd); err != nil {
		return fmt.Errorf("write %s: %w: %s", dest, err, errOut.String())
	}
	return nil
}

func (d *Docker) writeFileFrom(ctx context.Context, name, dest, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var errOut limitedBuffer
	errOut.max = 4096
	if err := d.exec(ctx, name, f, io.Discard, &errOut, "sh", "-c", "cat > "+dest); err != nil {
		return fmt.Errorf("write %s: %w: %s", dest, err, errOut.String())
	}
	return nil
}

func (d *Docker) copyArtifacts(ctx context.Context, name, artifactDir string, spec languageSpec, checker bool) error {
	switch spec.artifact {
	case artifactBinary:
		file := filepath.Join(artifactDir, "main")
		dest := "/work/main"
		if checker {
			file = filepath.Join(artifactDir, "checker")
			dest = "/work/checker"
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		return d.writeFile(ctx, name, dest, data, 0o755)
	case artifactSource:
		data, err := os.ReadFile(filepath.Join(artifactDir, "main.py"))
		if err != nil {
			return err
		}
		return d.writeFile(ctx, name, "/work/main.py", data, 0o644)
	case artifactClasses:
		data, err := tarDir(filepath.Join(artifactDir, "classes"))
		if err != nil {
			return err
		}
		var errOut limitedBuffer
		errOut.max = 4096
		if err := d.exec(ctx, name, bytes.NewReader(data), io.Discard, &errOut,
			"sh", "-c", "mkdir -p /work/classes && tar xf - --no-same-owner -C /work/classes"); err != nil {
			return fmt.Errorf("extract Java classes: %w: %s", err, errOut.String())
		}
		return nil
	}
	return fmt.Errorf("unknown artifact kind %q", spec.artifact)
}

func (d *Docker) copyOutFile(ctx context.Context, name, src, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	var errOut limitedBuffer
	errOut.max = 4096
	copyErr := d.exec(ctx, name, nil, f, &errOut, "cat", src)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("copy %s out: %w: %s", src, copyErr, errOut.String())
	}
	return closeErr
}

func (d *Docker) copyOutDir(ctx context.Context, name, src, dest string) error {
	var buf bytes.Buffer
	if err := d.exec(ctx, name, nil, &buf, io.Discard, "tar", "cf", "-", "-C", src, "."); err != nil {
		return fmt.Errorf("copy %s out: %w", src, err)
	}
	return extractTar(buf.Bytes(), dest)
}

func (d *Docker) compileFlags() []string {
	return []string{
		"--label", "kapoj.judge=1",
		"--label", "kapoj.judge.instance=" + d.instance,
		"--network", "none",
		"--cpus", "1",
		"--memory", "1g",
		"--pids-limit", "128",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "65532:65532",
		"--read-only",
		"--tmpfs", "/work:rw,exec,nosuid,size=256m,mode=1777",
		"--tmpfs", "/tmp:rw,exec,nosuid,size=512m,mode=1777",
		"--tmpfs", "/out:rw,noexec,nosuid,size=8m,mode=1777",
		"--workdir", "/work",
	}
}

func (d *Docker) runFlags(memoryMB, outputLimit int) []string {
	if memoryMB < runMemoryFloor {
		memoryMB = runMemoryFloor
	}
	memory := fmt.Sprintf("%dm", memoryMB)
	return []string{
		"--label", "kapoj.judge=1",
		"--label", "kapoj.judge.instance=" + d.instance,
		"--network", "none",
		"--cpus", "1",
		"--memory", memory,
		"--memory-swap", memory,
		"--pids-limit", "64",
		"--cap-drop", "ALL",
		"--cap-add", "SETUID",
		"--cap-add", "SETGID",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--tmpfs", "/work:rw,exec,nosuid,size=64m,mode=0755",
		"--tmpfs", "/tmp:rw,exec,nosuid,size=64m,mode=1777",
		"--tmpfs", "/out:rw,noexec,nosuid," + outTmpfsSize(outputLimit) + ",mode=0755",
		"--workdir", "/work",
	}
}

func runScript(command []string, outputLimit int) string {
	fileLimit := outputFileBlocks(outputLimit)
	return `#!/bin/sh
ulimit -f ` + strconv.Itoa(fileLimit) + `
for p in /proc/[0-9]*; do
  [ -r "$p/status" ] || continue
  while read -r key value rest; do
    if [ "$key" = "Uid:" ]; then
      set -- $value $rest
      [ "$1" = "65532" ] && kill -9 "${p##*/}" 2>/dev/null || true
      break
    fi
  done < "$p/status"
done
rm -f /out/stdout /out/stderr /out/time_ms /out/cpu_ms /out/memory_kb /out/oom_kill /out/oom_before /out/exit_code /out/timed_out /out/memory_peak_bytes 2>/dev/null
rm -rf /tmp/* /tmp/.[!.]* /tmp/..?* 2>/dev/null
rm -rf /dev/shm/* /dev/shm/.[!.]* /dev/shm/..?* 2>/dev/null
cd /tmp

time_limit_ms=$(cat /work/time_limit_ms 2>/dev/null || echo 1000)
case "$time_limit_ms" in
  ''|*[!0-9]*) time_limit_ms=1000 ;;
esac
[ "$time_limit_ms" -gt 0 ] || time_limit_ms=1000
sec=$((time_limit_ms / 1000))
frac=$((time_limit_ms % 1000))
if [ "$frac" -eq 0 ]; then
  limit="${sec}s"
else
  frac=$(printf '%03d' "$frac")
  limit="${sec}.${frac}s"
fi

oom_before=0
if [ -r /sys/fs/cgroup/memory.events ]; then
  while read -r key value; do
    [ "$key" = "oom_kill" ] && oom_before=$value
  done < /sys/fs/cgroup/memory.events
fi
echo "$oom_before" > /out/oom_before

cpu_before=0
cpu_ok=0
if [ -r /sys/fs/cgroup/cpu.stat ]; then
  while read -r key value; do
    if [ "$key" = "usage_usec" ]; then
      cpu_before=$value
      cpu_ok=1
    fi
  done < /sys/fs/cgroup/cpu.stat
fi

# cgroup v2 memory.peak is the exact per-round high-water mark when writable.
# Docker mounts /sys/fs/cgroup read-only by default, so fall back to sampling
# memory.current in the root wrapper; the sampled peak is reset every round.
reset_peak=0
if echo 0 2>/dev/null > /sys/fs/cgroup/memory.peak; then
  reset_peak=1
else
  : > /out/memory_peak_bytes
  (
    peak=0
    while :; do
      current=$(cat /sys/fs/cgroup/memory.current 2>/dev/null) || break
      if [ "$current" -gt "$peak" ]; then
        peak=$current
        echo "$peak" > /out/memory_peak_bytes
      fi
      sleep 0.01
    done
  ) &
  sampler=$!
fi

start=$(date +%s%N)
timeout --signal=KILL "$limit" setpriv --reuid=65532 --regid=65532 --clear-groups -- ` + strings.Join(command, " ") + ` < /work/input.txt > /out/stdout 2> /out/stderr
status=$?
end=$(date +%s%N)
elapsed=$(( (end - start) / 1000000 ))

echo "$status" > /out/exit_code
echo "$elapsed" > /out/time_ms
if [ "$status" -eq 124 ] || [ "$elapsed" -ge "$time_limit_ms" ]; then
  echo 1 > /out/timed_out
else
  echo 0 > /out/timed_out
fi

oom_after=0
if [ -r /sys/fs/cgroup/memory.events ]; then
  while read -r key value; do
    [ "$key" = "oom_kill" ] && oom_after=$value
  done < /sys/fs/cgroup/memory.events
fi
echo $(( oom_after - oom_before )) > /out/oom_kill

if [ "$cpu_ok" -eq 1 ] && [ -r /sys/fs/cgroup/cpu.stat ]; then
  cpu_after=0
  while read -r key value; do
    [ "$key" = "usage_usec" ] && cpu_after=$value
  done < /sys/fs/cgroup/cpu.stat
  if [ "$cpu_after" -ge "$cpu_before" ]; then
    echo $(( (cpu_after - cpu_before) / 1000 )) > /out/cpu_ms
  else
    echo -1 > /out/cpu_ms
  fi
else
  echo -1 > /out/cpu_ms
fi

if [ "$reset_peak" -eq 1 ]; then
  peak=$(cat /sys/fs/cgroup/memory.peak 2>/dev/null || echo 0)
else
  kill "$sampler" 2>/dev/null || true
  wait "$sampler" 2>/dev/null || true
  peak=$(cat /out/memory_peak_bytes 2>/dev/null || echo 0)
fi
echo $(( peak / 1024 )) > /out/memory_kb
exit 0
`
}

// roundCleanupScript kills a user process left behind by an interrupted
// docker exec and clears per-round state before retrying the round.
func roundCleanupScript() string {
	return `for p in /proc/[0-9]*; do
  [ -r "$p/status" ] || continue
  while read -r key value rest; do
    if [ "$key" = "Uid:" ]; then
      set -- $value $rest
      [ "$1" = "65532" ] && kill -9 "${p##*/}" 2>/dev/null || true
      break
    fi
  done < "$p/status"
done
rm -rf /tmp/* /tmp/.[!.]* /tmp/..?* 2>/dev/null || true
rm -rf /dev/shm/* /dev/shm/.[!.]* /dev/shm/..?* 2>/dev/null || true
rm -f /out/stdout /out/stderr /out/time_ms /out/cpu_ms /out/memory_kb /out/oom_kill /out/oom_before /out/exit_code /out/timed_out /out/memory_peak_bytes 2>/dev/null || true
`
}

func containerName() string { return "kapoj-judge-" + randomHex(6) }

// effectiveOutputLimit normalizes a per-run output limit to at least 1 MiB.
func effectiveOutputLimit(limit int) int {
	if limit <= 0 {
		return defaultOutputLimit
	}
	return limit
}

// outputFileLimit returns how many bytes /out must be able to hold for a run:
// the requested output limit plus headroom for stderr and measurement files,
// never below the tmpfs floor.
func outputFileLimit(limit int) int {
	size := effectiveOutputLimit(limit) + outputOverhead
	if size < minOutTmpfs {
		size = minOutTmpfs
	}
	return size
}

// outTmpfsSize returns the size= argument for the /out tmpfs mount, rounded up
// to whole MiB.
func outTmpfsSize(limit int) string {
	size := outputFileLimit(limit)
	return "size=" + strconv.Itoa((size+(1<<20)-1)/(1<<20)) + "m"
}

// outputFileBlocks returns the ulimit -f argument for a run. It expresses the
// /out capacity in 512-byte blocks, the smallest unit a shell may use, so a
// shell interpreting blocks as 1024 bytes still ends up with at least the
// requested headroom.
func outputFileBlocks(limit int) int {
	return (outputFileLimit(limit) + 511) / 512
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max > 0 && b.Len() < b.max {
		remaining := b.max - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return len(p), nil
}

func tarDir(dir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		// Container tar runs without CAP_CHOWN; do not ask it to restore the
		// host's numeric owner.
		hdr.Uid, hdr.Gid = 0, 0
		hdr.Uname, hdr.Gname = "", ""
		if entry.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		_ = tw.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extractTar(data []byte, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe tar entry %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
