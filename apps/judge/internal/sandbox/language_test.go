package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageSpecs(t *testing.T) {
	tests := []struct {
		language string
		source   string
		compile  []string
		artifact string
		run      []string
	}{
		{
			"c", "/work/main.c",
			[]string{"gcc", "-O2", "-std=c17", "-o", "/tmp/main", "/work/main.c"},
			artifactBinary,
			[]string{"/work/main"},
		},
		{
			"cpp", "/work/main.cpp",
			[]string{"g++", "-O2", "-std=c++17", "-o", "/tmp/main", "/work/main.cpp"},
			artifactBinary,
			[]string{"/work/main"},
		},
		{
			"python3", "/work/main.py",
			[]string{"env", "PYTHONPYCACHEPREFIX=/tmp/pycache", "python3", "-m", "py_compile", "/work/main.py"},
			artifactSource,
			[]string{"python3", "/work/main.py"},
		},
		{
			"java", "/work/Main.java",
			[]string{"javac", "-encoding", "UTF-8", "-d", "/tmp/classes", "/work/Main.java"},
			artifactClasses,
			[]string{"java", "-Xmx192m", "-cp", "/work/classes", "Main"},
		},
		{
			"go", "/work/main.go",
			[]string{"env", "GO111MODULE=off", "GOCACHE=/tmp/gocache", "GOPATH=/tmp/gopath", "CGO_ENABLED=0",
				"go", "build", "-o", "/tmp/main", "/work/main.go"},
			artifactBinary,
			[]string{"/work/main"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			spec, err := languageFor(tt.language, false, 256)
			require.NoError(t, err)
			assert.Equal(t, tt.source, spec.sourcePath)
			assert.Equal(t, tt.compile, spec.compile)
			assert.Equal(t, tt.artifact, spec.artifact)
			assert.Equal(t, tt.run, spec.run)
		})
	}
}

func TestJavaHeapFollowsMemoryLimit(t *testing.T) {
	spec, err := languageFor("java", false, 64)
	require.NoError(t, err)
	assert.Contains(t, spec.run, "-Xmx16m")

	spec, err = languageFor("java", false, 512)
	require.NoError(t, err)
	assert.Contains(t, spec.run, "-Xmx448m")
}

func TestCheckerSpec(t *testing.T) {
	spec, err := languageFor("cpp", true, 256)
	require.NoError(t, err)
	assert.Equal(t, "/work/checker.cpp", spec.sourcePath)
	assert.Equal(t, artifactBinary, spec.artifact)
	assert.Equal(t, []string{"/work/checker", "/work/input.txt", "/work/output.txt", "/work/answer.txt"}, spec.run)
}

func TestUnknownLanguage(t *testing.T) {
	_, err := languageFor("rust", false, 256)
	assert.Error(t, err)
}
