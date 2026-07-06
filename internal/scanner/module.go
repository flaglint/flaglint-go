package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// findModule searches scanRoot and its ancestor directories for the
// nearest go.mod, returning its declared module path and the directory
// containing it (the module root). Cross-package factory-function
// resolution (factory.go) needs this to compute real Go import paths; if
// no go.mod is found, that resolution is silently skipped — same-file and
// same-package (by directory) resolution are unaffected, since those don't
// need an import path at all.
//
// Does not handle nested go.mod files within the scanned tree (a
// monorepo with independent submodules) — files under a submodule would
// get an import path computed relative to the outer module, which is
// wrong. This is a known, narrow limitation: the failure mode is a missed
// cross-package resolution (false negative), never an incorrect match,
// since a wrong import path just won't equal any real import string.
func findModule(scanRoot string) (modulePath, moduleRoot string, ok bool) {
	dir := scanRoot
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			if mp := parseModuleDirective(string(data)); mp != "" {
				return mp, dir, true
			}
			return "", "", false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

func parseModuleDirective(gomod string) string {
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// packageImportPath computes the Go import path for a package declared in
// fileDir, given the enclosing module's path and root directory.
func packageImportPath(modulePath, moduleRoot, fileDir string) (string, error) {
	rel, err := filepath.Rel(moduleRoot, fileDir)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return modulePath, nil
	}
	return modulePath + "/" + rel, nil
}
