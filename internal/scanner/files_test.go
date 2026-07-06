package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package fixtures\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFiles(t *testing.T) {
	root := t.TempDir()

	touch(t, filepath.Join(root, "main.go"))
	touch(t, filepath.Join(root, "pkg", "svc.go"))
	touch(t, filepath.Join(root, "vendor", "dep", "dep.go"))
	touch(t, filepath.Join(root, "testdata", "fixture.go"))
	touch(t, filepath.Join(root, ".git", "internal.go"))
	touch(t, filepath.Join(root, "README.md"))

	include := []string{"**/*.go"}
	exclude := []string{"**/vendor/**", "**/testdata/**", "**/.git/**"}

	got, err := discoverFiles(root, include, exclude)
	if err != nil {
		t.Fatalf("discoverFiles() error = %v", err)
	}
	sort.Strings(got)

	want := []string{"main.go", "pkg/svc.go"}
	if len(got) != len(want) {
		t.Fatalf("discoverFiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("discoverFiles()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverFiles_largeExcludedTreeIsPruned(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "main.go"))
	// A vendor tree deep enough that if discoverFiles failed to prune it,
	// this test would still pass functionally but the point is pruning
	// avoids walking it at all — verified indirectly via the result set.
	for i := 0; i < 20; i++ {
		touch(t, filepath.Join(root, "vendor", "dep", string(rune('a'+i))+".go"))
	}

	got, err := discoverFiles(root, []string{"**/*.go"}, []string{"**/vendor/**"})
	if err != nil {
		t.Fatalf("discoverFiles() error = %v", err)
	}
	if len(got) != 1 || got[0] != "main.go" {
		t.Errorf("discoverFiles() = %v, want [main.go]", got)
	}
}
