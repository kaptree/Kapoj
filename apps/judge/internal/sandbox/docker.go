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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	compileTimeout = 30 * time.Second
	containerGrace = 3 * time.Second
	runMemoryFloor = 16
)

type Docker struct {
	bin    string
	images map[string]string
}

func NewDocker(bin string, images map[string]string) *Docker {
	return &Docker{bin: bin, images: images}
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
	if err := d.start(compileCtx, name, image, compileFlags()); err != nil {
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
	name := containerName()
	if err := d.start(ctx, name, image, runFlags(req.MemoryLimitMB)); err != nil {
		d.remove(name)
		return RunResult{}, err
	}
	defer d.remove(name)

	if err := d.copyArtifacts(ctx, name, req.ArtifactDir, spec, checker); err != nil {
		return RunResult{}, err
	}
	if err := d.writeFileFrom(ctx, name, "/work/input.txt", req.InputPath); err != nil {
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
	script := runScript(spec.run)
	if err := d.writeFile(ctx, name, "/work/run.sh", []byte(script), 0o644); err != nil {
		return RunResult{}, err
	}

	timeLimit := req.TimeLimitMS
	if timeLimit <= 0 {
		timeLimit = 1000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeLimit)*time.Millisecond+containerGrace)
	defer cancel()
	var out, errOut bytes.Buffer
	err = d.exec(runCtx, name, nil, &out, &errOut, "sh", "/work/run.sh")
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return RunResult{TimedOut: true, ExitCode: -1, TimeMS: timeLimit + 1}, nil
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return RunResult{}, fmt.Errorf("run container: %w: %s", err, errOut.String())
		}
		exitCode = exitErr.ExitCode()
	}

	limit := req.OutputLimitBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	output, truncated, err := d.readOut(ctx, name, "/out/stdout", limit)
	if err != nil {
		return RunResult{}, err
	}
	stderr, _, err := d.readOut(ctx, name, "/out/stderr", 4096)
	if err != nil {
		return RunResult{}, err
	}
	timeMS := d.readInt(ctx, name, "/out/time_ms")
	memoryKB := d.readInt(ctx, name, "/out/memory_kb")
	oomKills := d.readInt(ctx, name, "/out/oom_kill")
	return RunResult{
		Output: output, Stderr: stderr, ExitCode: exitCode,
		OOMKilled: oomKills > 0 || exitCode == 137,
		TimeMS:    timeMS, MemoryKB: memoryKB, OutputTruncated: truncated,
	}, nil
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
		return d.exec(ctx, name, bytes.NewReader(data), io.Discard, &errOut,
			"sh", "-c", "mkdir -p /work/classes && tar xf - -C /work/classes")
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

func (d *Docker) readOut(ctx context.Context, name, path string, limit int) ([]byte, bool, error) {
	if limit <= 0 {
		limit = 1 << 20
	}
	var buf bytes.Buffer
	err := d.exec(ctx, name, nil, &buf, io.Discard, "sh", "-c",
		fmt.Sprintf("head -c %d %s", limit+1, path))
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	truncated := buf.Len() > limit
	if truncated {
		return buf.Bytes()[:limit], true, nil
	}
	return buf.Bytes(), false, nil
}

func (d *Docker) readInt(ctx context.Context, name, path string) int {
	data, _, err := d.readOut(ctx, name, path, 64)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}

func compileFlags() []string {
	return []string{
		"--label", "kapoj.judge=1",
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

func runFlags(memoryMB int) []string {
	if memoryMB < runMemoryFloor {
		memoryMB = runMemoryFloor
	}
	memory := fmt.Sprintf("%dm", memoryMB)
	return []string{
		"--label", "kapoj.judge=1",
		"--network", "none",
		"--cpus", "1",
		"--memory", memory,
		"--memory-swap", memory,
		"--pids-limit", "64",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "65532:65532",
		"--read-only",
		"--tmpfs", "/work:rw,exec,nosuid,size=64m,mode=1777",
		"--tmpfs", "/tmp:rw,exec,nosuid,size=64m,mode=1777",
		"--tmpfs", "/out:rw,noexec,nosuid,size=8m,mode=1777",
		"--workdir", "/work",
	}
}

func runScript(command []string) string {
	return `#!/bin/sh
ulimit -f 4096
oom_before=0
if [ -r /sys/fs/cgroup/memory.events ]; then
  while read -r key value; do
    [ "$key" = "oom_kill" ] && oom_before=$value
  done < /sys/fs/cgroup/memory.events
fi
start=$(date +%s%N)
` + strings.Join(command, " ") + ` < /work/input.txt > /out/stdout 2> /out/stderr
status=$?
end=$(date +%s%N)
echo $(( (end - start) / 1000000 )) > /out/time_ms
oom_after=0
if [ -r /sys/fs/cgroup/memory.events ]; then
  while read -r key value; do
    [ "$key" = "oom_kill" ] && oom_after=$value
  done < /sys/fs/cgroup/memory.events
fi
echo $(( oom_after - oom_before )) > /out/oom_kill
if [ -r /sys/fs/cgroup/memory.peak ]; then
  peak=$(cat /sys/fs/cgroup/memory.peak)
  echo $(( peak / 1024 )) > /out/memory_kb
else
  echo 0 > /out/memory_kb
fi
exit $status
`
}

func containerName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "kapoj-judge-" + hex.EncodeToString(b)
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
