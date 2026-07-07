// Package typecheck loads a Go module with real go/types information via
// golang.org/x/tools/go/packages — the foundation for flaglint-go's opt-in
// --strict-types pass (see docs/adr/005-strict-types-pass.md). It knows
// nothing about LaunchDarkly or flag detection; it only answers "what are
// this module's packages, and what does the type checker know about them."
// The actual identity-resolution logic that consumes this lives in
// internal/scanner, which already owns every other piece of "what counts
// as an SDK usage" domain knowledge.
package typecheck

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// LoadFailure records one package that could not be loaded or did not
// type-check cleanly. This is never fatal to the caller — see Load's doc
// comment — it exists purely for the caller to surface as a warning.
type LoadFailure struct {
	PkgPath string
	Reason  string
}

const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedSyntax

// Load loads every package in dir (via the "./..." pattern) with full type
// information. Per ADR 005's fail-soft design, a package that fails to
// load or has type errors is never treated as fatal to the whole call — it
// is reported as a LoadFailure and simply excluded from the returned
// slice, so a caller can still make use of whatever packages *did*
// type-check cleanly. Load only returns a non-nil error for something that
// prevents loading from happening at all (e.g. dir isn't a Go module).
func Load(dir string) ([]*packages.Package, []LoadFailure, error) {
	// The actual type-checking behind NeedTypes/NeedTypesInfo runs
	// in-process (unlike package discovery, which packages.Load shells out
	// for) — so it's *this process's* own GODEBUG that governs it, not
	// packages.Config.Env, which only reaches the subprocesses Load spawns
	// for discovery. os.Setenv (rather than just building a modified Env
	// slice) is what's needed to actually affect it — Go's runtime keeps
	// GODEBUG's internal cache in sync with os.Setenv calls specifically
	// for this purpose. See withGotypesalias's doc comment for why this
	// override exists at all. Mutating process-wide state here is only
	// safe because Load runs synchronously, once, near the start of a CLI
	// invocation — not a pattern to repeat somewhere with concurrent
	// unrelated callers.
	if updated, changed := withGotypesalias(os.Getenv("GODEBUG")); changed {
		// Setenv on a literal, well-formed key like "GODEBUG" can only
		// fail for a key containing "=" or a NUL byte — never true here —
		// so there's nothing meaningful to do with an error even if one
		// somehow occurred.
		_ = os.Setenv("GODEBUG", updated)
	}

	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  dir,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, nil, fmt.Errorf("load packages under %s: %w", dir, err)
	}

	var loaded []*packages.Package
	var failures []LoadFailure
	for _, pkg := range pkgs {
		// IllTyped covers errors in the package itself *or any dependency*
		// (per packages.Package's doc comment) — excluded too, not just
		// pkg.Errors, since type info built on an ill-typed dependency
		// can't be trusted as proof of anything (ADR 005: fail soft, but
		// never guess).
		if len(pkg.Errors) == 0 && !pkg.IllTyped {
			loaded = append(loaded, pkg)
			continue
		}
		reason := "package has unresolved type errors"
		if len(pkg.Errors) > 0 {
			reason = pkg.Errors[0].Error()
		}
		failures = append(failures, LoadFailure{PkgPath: pkg.PkgPath, Reason: reason})
	}

	return loaded, failures, nil
}

// withGotypesalias returns the GODEBUG value that should be in effect for
// type-checking (godebug, changed=true) — current with gotypesalias forced
// to 1 (real type aliases, not the pre-1.24 desugared representation) — or
// (current, false) if the caller's environment already specifies a
// gotypesalias setting explicitly, which is left alone rather than
// overridden.
//
// Found empirically: flaglint-go's own go.mod declares an older go version
// than a scanned target's, which makes the *compiled binary's* embedded
// GODEBUG default (not the target module's own go version) govern how
// go/types represents the target's type aliases. On a target using Go
// 1.24+ generic type aliases, that mismatch makes every package that
// (transitively) touches one fail to type-check with "generic type alias
// requires GODEBUG=gotypesalias=1 or unset" — confirmed directly against a
// real, large codebase (weaviate/weaviate): 321 of its packages failed
// without this override and zero failed with it. This is a property of
// the *analyzing* binary, unrelated to anything in the scanned module's
// own go.mod, so it must be forced here rather than left for a user to
// discover and work around themselves.
func withGotypesalias(current string) (godebug string, changed bool) {
	if strings.Contains(current, "gotypesalias=") {
		return current, false
	}
	if current == "" {
		return "gotypesalias=1", true
	}
	return current + ",gotypesalias=1", true
}
