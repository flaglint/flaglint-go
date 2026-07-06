package scanner

import (
	"io/fs"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

// discoverFiles walks root and returns paths (relative to root, forward
// slashes) matching any include pattern and no exclude pattern. Directories
// whose children would all be excluded are pruned rather than descended
// into, so a large excluded tree (e.g. vendor/) is never walked.
func discoverFiles(root string, include, exclude []string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel != "." && dirWouldBeExcluded(rel, exclude) {
				return filepath.SkipDir
			}
			return nil
		}

		if matchesAny(rel, exclude) || !matchesAny(rel, include) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

// dirWouldBeExcluded reports whether a synthetic child of dir would match
// an exclude pattern — the standard way to test a "**/x/**"-style directory
// pattern without requiring an actual child to exist yet.
func dirWouldBeExcluded(dir string, exclude []string) bool {
	return matchesAny(dir+"/__flaglint_probe__", exclude)
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := doublestar.Match(p, path); ok {
			return true
		}
	}
	return false
}
