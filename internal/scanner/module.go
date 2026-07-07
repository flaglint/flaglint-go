package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// moduleInfo is the result of a go.mod search for one directory, cached
// per-directory (see findModule) so files sharing the same nearest go.mod
// — the common case, one module per scan — don't each re-walk and re-read
// the filesystem from scratch.
type moduleInfo struct {
	modulePath string
	moduleRoot string
	ok         bool
}

// findModule searches dir and its ancestor directories for the nearest
// go.mod, returning its declared module path and the directory containing
// it (the module root). Cross-package factory-function resolution
// (factory.go) needs this to compute real Go import paths; if no go.mod
// is found, that resolution is silently skipped for files under dir —
// same-file and same-package (by directory) resolution are unaffected,
// since those don't need an import path at all.
//
// Callers pass a shared cache (one per scan) and call this once per file
// directory rather than once for the whole scan root — searching upward
// from *each file's own directory* is what correctly handles a monorepo
// with independent nested submodules (issue #17): a file under a
// submodule finds that submodule's own, nearer go.mod before ever
// reaching the outer one, rather than always resolving against whichever
// go.mod happens to be nearest the scan root. Every directory visited
// during a search (whether the search started there or merely passed
// through it on the way up) is cached with the final result, so a
// same-module sibling file's lookup is a single map read.
func findModule(dir string, cache map[string]moduleInfo) (modulePath, moduleRoot string, ok bool) {
	var visited []string
	d := dir
	for {
		if info, cached := cache[d]; cached {
			for _, v := range visited {
				cache[v] = info
			}
			return info.modulePath, info.moduleRoot, info.ok
		}
		visited = append(visited, d)

		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			info := moduleInfo{}
			if mp := parseModuleDirective(string(data)); mp != "" {
				info = moduleInfo{modulePath: mp, moduleRoot: d, ok: true}
			}
			for _, v := range visited {
				cache[v] = info
			}
			return info.modulePath, info.moduleRoot, info.ok
		}

		parent := filepath.Dir(d)
		if parent == d {
			info := moduleInfo{}
			for _, v := range visited {
				cache[v] = info
			}
			return "", "", false
		}
		d = parent
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
