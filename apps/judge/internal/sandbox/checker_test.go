package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerSandboxChecker(t *testing.T) {
	d := newTestDocker(t)
	checker := `#include <fstream>
#include <iterator>
#include <string>
int main(int argc, char** argv) {
  std::ifstream u(argv[2]), a(argv[3]);
  std::string us((std::istreambuf_iterator<char>(u)), {});
  std::string as((std::istreambuf_iterator<char>(a)), {});
  return us == as ? 0 : 1;
}`
	ctx := context.Background()
	compiled, err := d.Compile(ctx, CompileRequest{Language: "cpp", Source: []byte(checker), Checker: true})
	require.NoError(t, err)
	require.False(t, compiled.Failed, compiled.ErrorOutput)
	defer d.Cleanup(compiled.ArtifactDir)

	dir := t.TempDir()
	input := filepath.Join(dir, "input.txt")
	answer := filepath.Join(dir, "answer.txt")
	good := filepath.Join(dir, "good.txt")
	bad := filepath.Join(dir, "bad.txt")
	require.NoError(t, os.WriteFile(input, []byte("1 2\n"), 0o644))
	require.NoError(t, os.WriteFile(answer, []byte("3\n"), 0o644))
	require.NoError(t, os.WriteFile(good, []byte("3\n"), 0o644))
	require.NoError(t, os.WriteFile(bad, []byte("4\n"), 0o644))

	run, err := d.Run(ctx, RunRequest{
		Language: "cpp", ArtifactDir: compiled.ArtifactDir, InputPath: input,
		Checker:          &CheckerFiles{AnswerPath: answer, OutputPath: good},
		TimeLimitMS:      5000,
		MemoryLimitMB:    256,
		OutputLimitBytes: 4096,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode)

	run, err = d.Run(ctx, RunRequest{
		Language: "cpp", ArtifactDir: compiled.ArtifactDir, InputPath: input,
		Checker:          &CheckerFiles{AnswerPath: answer, OutputPath: bad},
		TimeLimitMS:      5000,
		MemoryLimitMB:    256,
		OutputLimitBytes: 4096,
	})
	require.NoError(t, err)
	assert.NotZero(t, run.ExitCode)
	assertNoJudgeContainers(t)
}
