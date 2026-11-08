package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dockerImages() map[string]string {
	return map[string]string{
		"c":       "gcc:14",
		"cpp":     "gcc:14",
		"python3": "python:3.12-slim",
		"java":    "eclipse-temurin:21-jdk",
		"go":      "golang:1.26",
	}
}

func newTestDocker(t *testing.T) *Docker {
	t.Helper()
	if os.Getenv("KAPOJ_DOCKER_TESTS") != "1" {
		t.Skip("KAPOJ_DOCKER_TESTS is not 1; skipping Docker sandbox test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker binary not found")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon unavailable")
	}
	return NewDocker("docker", dockerImages())
}

func compileAndRun(t *testing.T, d *Docker, language, code, input string, timeMS, memoryMB, outputLimit int) (RunResult, CompileResult) {
	t.Helper()
	ctx := context.Background()
	compiled, err := d.Compile(ctx, CompileRequest{Language: language, Source: []byte(code)})
	require.NoError(t, err)
	require.False(t, compiled.Failed, compiled.ErrorOutput)
	if compiled.ArtifactDir != "" {
		defer d.Cleanup(compiled.ArtifactDir)
	}
	in := t.TempDir() + "/input.txt"
	require.NoError(t, os.WriteFile(in, []byte(input), 0o644))
	run, err := d.Run(ctx, RunRequest{
		Language: language, ArtifactDir: compiled.ArtifactDir, InputPath: in,
		TimeLimitMS: timeMS, MemoryLimitMB: memoryMB, OutputLimitBytes: outputLimit,
	})
	require.NoError(t, err)
	return run, compiled
}

func TestDockerSandboxVerdicts(t *testing.T) {
	d := newTestDocker(t)
	ac := "#include <iostream>\nint main(){int a,b;std::cin>>a>>b;std::cout<<a+b<<\"\\n\";}\n"

	run, _ := compileAndRun(t, d, "cpp", ac, "1 2\n", 1000, 256, 1<<20)
	assert.False(t, run.TimedOut)
	assert.False(t, run.OOMKilled)
	assert.Equal(t, 0, run.ExitCode)
	assert.Equal(t, "3\n", string(run.Output))
	assert.GreaterOrEqual(t, run.TimeMS, 0)
	assert.Greater(t, run.MemoryKB, 0)

	run, _ = compileAndRun(t, d, "cpp", "int main(){return 1;}", "", 1000, 256, 1<<20)
	assert.Equal(t, 1, run.ExitCode)
	assert.False(t, run.OOMKilled)

	run, _ = compileAndRun(t, d, "cpp", "int main(){for(;;);}", "", 200, 256, 1<<20)
	assert.True(t, run.TimedOut)

	run, _ = compileAndRun(t, d, "cpp",
		"#include <cstdlib>\n#include <cstring>\nint main(){for(;;){char*p=(char*)malloc(16<<20);memset(p,1,16<<20);}}",
		"", 5000, 64, 1<<20)
	assert.True(t, run.OOMKilled || run.ExitCode == 137, "exit=%d", run.ExitCode)

	compiled, err := d.Compile(context.Background(), CompileRequest{Language: "cpp", Source: []byte("int main({")})
	require.NoError(t, err)
	assert.True(t, compiled.Failed)
	assert.Contains(t, compiled.ErrorOutput, "error")

	assertNoJudgeContainers(t)
}

func TestDockerSandboxLanguages(t *testing.T) {
	d := newTestDocker(t)
	tests := []struct {
		language string
		code     string
		memoryMB int
	}{
		{"python3", "a,b=map(int,input().split())\nprint(a+b)\n", 256},
		{"java", `import java.util.Scanner;
public class Main { public static void main(String[] args) { Scanner s = new Scanner(System.in); System.out.println(s.nextInt() + s.nextInt()); } }`, 512},
		{"go", `package main
import "fmt"
func main() { var a, b int; fmt.Scan(&a, &b); fmt.Println(a + b) }
`, 256},
		{"c", "#include <stdio.h>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\\n\",a+b);}\n", 256},
	}
	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			run, _ := compileAndRun(t, d, tt.language, tt.code, "1 2\n", 5000, tt.memoryMB, 1<<20)
			assert.Equal(t, 0, run.ExitCode, string(run.Stderr))
			assert.Equal(t, "3\n", string(run.Output))
		})
	}
	assertNoJudgeContainers(t)
}

func TestDockerSandboxOutputLimit(t *testing.T) {
	d := newTestDocker(t)
	run, _ := compileAndRun(t, d, "cpp",
		`#include <cstdio>
int main(){for(int i=0;i<2*1024*1024;i++) putchar('a');}`,
		"", 5000, 256, 1024)
	assert.True(t, run.OutputTruncated)
	assert.LessOrEqual(t, len(run.Output), 1024)
	assertNoJudgeContainers(t)
}

// TestDockerSandboxOutputLimitLarge proves the sandbox honors a >4 MiB output
// limit. A producer that prints 7 MiB with OutputLimitBytes=6<<20 must return
// exactly 6 MiB and flag truncation. The old code capped the real output at
// ~4 MiB via `ulimit -f 4096` and an 8 MiB /out tmpfs, so the run would have
// died before ever reaching the requested 6 MiB.
func TestDockerSandboxOutputLimitLarge(t *testing.T) {
	d := newTestDocker(t)
	run, _ := compileAndRun(t, d, "cpp",
		`#include <cstdio>
int main(){for(int i=0;i<7*1024*1024;i++) putchar('a');}`,
		"", 10000, 256, 6<<20)
	assert.True(t, run.OutputTruncated)
	assert.Equal(t, 6<<20, len(run.Output))
	assertNoJudgeContainers(t)
}

func assertNoJudgeContainers(t *testing.T) {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a", "--filter", "label=kapoj.judge=1", "--format", "{{.Names}}").Output()
	require.NoError(t, err)
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("judge containers left behind: %s", out)
	}
}

func TestDockerSandboxCompileTimeoutIsError(t *testing.T) {
	d := newTestDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := d.Compile(ctx, CompileRequest{Language: "cpp", Source: []byte("int main(){}")})
	assert.Error(t, err)
}

func TestDockerSandboxParentDeadlineIsSystemError(t *testing.T) {
	d := newTestDocker(t)
	compiled, err := d.Compile(context.Background(), CompileRequest{
		Language: "cpp", Source: []byte("int main(){for(;;);}"),
	})
	require.NoError(t, err)
	defer d.Cleanup(compiled.ArtifactDir)
	input := t.TempDir() + "/input.txt"
	require.NoError(t, os.WriteFile(input, []byte(""), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err = d.Run(ctx, RunRequest{
		Language: "cpp", ArtifactDir: compiled.ArtifactDir, InputPath: input,
		TimeLimitMS: 10000, MemoryLimitMB: 256, OutputLimitBytes: 1024,
	})
	assert.Error(t, err, "a parent deadline must not become a student TLE")
	assertNoJudgeContainers(t)
}

func TestDockerSweepRemovesOnlyThisInstance(t *testing.T) {
	d := newTestDocker(t)
	name := "kapoj-sweep-probe-" + d.instance
	flags := []string{
		"--label", "kapoj.judge=1",
		"--label", "kapoj.judge.instance=" + d.instance,
		"--network", "none",
	}
	require.NoError(t, d.start(context.Background(), name, "gcc:14", flags))
	d.Sweep(context.Background())
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name="+name, "--format", "{{.Names}}").Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)))
}

func TestDockerSandboxProgramCannotForgeTiming(t *testing.T) {
	d := newTestDocker(t)
	code := `#include <sys/stat.h>
#include <unistd.h>
int main(){mkdir("/out/time_ms",0700); usleep(2000000);}
`
	run, _ := compileAndRun(t, d, "cpp", code, "", 1000, 256, 1<<20)
	assert.False(t, run.TimedOut)
	assert.GreaterOrEqual(t, run.TimeMS, 1500, "the wrapper must measure the real runtime")
	assertNoJudgeContainers(t)
}
