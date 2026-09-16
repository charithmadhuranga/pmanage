package accelerator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// npuFS abstracts filesystem access so the portable NPU sources (ivpu,
// rockchip) are fixture-testable without a Linux host.
type npuFS interface {
	glob(pattern string) ([]string, error)
	readFile(path string) (string, error)
}

// realNpuFS reads from the live sysfs/devfreq tree.
type realNpuFS struct{}

func (realNpuFS) glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (realNpuFS) readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// cliRunner abstracts a subprocess so CLI-backed NPU sources (hailo,
// tegrastats) are fixture-testable.
type cliRunner interface {
	run(args ...string) (string, error)
}

// execRunner shells out to a resolved binary path (LookPath once).
type execRunner struct {
	name string
	path string
}

func (e *execRunner) run(args ...string) (string, error) {
	if e.path == "" {
		p, err := exec.LookPath(e.name)
		if err != nil {
			return "", err
		}
		e.path = p
	}
	cmd := exec.Command(e.path, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
