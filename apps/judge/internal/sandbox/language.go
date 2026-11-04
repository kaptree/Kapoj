package sandbox

import "fmt"

const (
	artifactBinary  = "binary"
	artifactClasses = "classes"
	artifactSource  = "source"
)

type languageSpec struct {
	sourcePath  string
	compile     []string
	artifact    string
	artifactSrc string // container path of the compiled binary
	run         []string
}

func languageFor(language string, checker bool, memoryMB int) (languageSpec, error) {
	if checker {
		return languageSpec{
			sourcePath:  "/work/checker.cpp",
			compile:     []string{"g++", "-O2", "-std=c++17", "-o", "/tmp/checker", "/work/checker.cpp"},
			artifact:    artifactBinary,
			artifactSrc: "/tmp/checker",
			run:         []string{"/work/checker", "/work/input.txt", "/work/output.txt", "/work/answer.txt"},
		}, nil
	}
	switch language {
	case "c":
		return languageSpec{
			sourcePath:  "/work/main.c",
			compile:     []string{"gcc", "-O2", "-std=c17", "-o", "/tmp/main", "/work/main.c"},
			artifact:    artifactBinary,
			artifactSrc: "/tmp/main",
			run:         []string{"/work/main"},
		}, nil
	case "cpp":
		return languageSpec{
			sourcePath:  "/work/main.cpp",
			compile:     []string{"g++", "-O2", "-std=c++17", "-o", "/tmp/main", "/work/main.cpp"},
			artifact:    artifactBinary,
			artifactSrc: "/tmp/main",
			run:         []string{"/work/main"},
		}, nil
	case "python3":
		return languageSpec{
			sourcePath: "/work/main.py",
			compile: []string{
				"env", "PYTHONPYCACHEPREFIX=/tmp/pycache",
				"python3", "-m", "py_compile", "/work/main.py",
			},
			artifact: artifactSource,
			run:      []string{"python3", "/work/main.py"},
		}, nil
	case "java":
		heap := memoryMB - 64
		if heap < 16 {
			heap = 16
		}
		return languageSpec{
			sourcePath:  "/work/Main.java",
			compile:     []string{"javac", "-encoding", "UTF-8", "-d", "/tmp/classes", "/work/Main.java"},
			artifact:    artifactClasses,
			artifactSrc: "/tmp/classes",
			run:         []string{"java", fmt.Sprintf("-Xmx%dm", heap), "-cp", "/work/classes", "Main"},
		}, nil
	case "go":
		return languageSpec{
			sourcePath: "/work/main.go",
			compile: []string{
				"env", "GO111MODULE=off", "GOCACHE=/tmp/gocache", "GOPATH=/tmp/gopath", "CGO_ENABLED=0",
				"go", "build", "-o", "/tmp/main", "/work/main.go",
			},
			artifact:    artifactBinary,
			artifactSrc: "/tmp/main",
			run:         []string{"/work/main"},
		}, nil
	default:
		return languageSpec{}, fmt.Errorf("unsupported language %q", language)
	}
}
