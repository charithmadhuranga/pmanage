package accelerator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureFS serves a virtual sysfs tree from a hierarchy rooted at a temp dir:
// map of relative path → contents. Glob is resolved against the same root.
type fixtureFS struct {
	root string
}

func newFixtureFS(t *testing.T, files map[string]string) *fixtureFS {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &fixtureFS{root: root}
}

func (f *fixtureFS) glob(pattern string) ([]string, error) {
	rel := strings.TrimPrefix(pattern, "/")
	full := filepath.Join(f.root, rel)
	matches, err := filepath.Glob(full)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, filepath.ToSlash(strings.TrimPrefix(m, f.root)))
	}
	return out, nil
}

func (f *fixtureFS) readFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(f.root, strings.TrimPrefix(path, "/")))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// fixtureCLI maps joined args → output for CLI-backed sources.
type fixtureCLI struct {
	script map[string]string
}

func (f *fixtureCLI) run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	if out, ok := f.script[key]; ok {
		return out, nil
	}
	return "", os.ErrNotExist
}
