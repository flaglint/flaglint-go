// Package baseline implements CI-adoption baseline mode: capture today's
// known findings' fingerprints, then fail only on fingerprints not in that
// set. File format, field types, and error/exit-code behavior match
// flaglint-js exactly — see docs/adr/003-cross-tool-contract.md.
package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// File is the on-disk baseline shape. Version is deliberately the string
// "1", not the number 1 — flaglint-js's own ADR 008 originally documented
// this as a number and had to be corrected to match the shipped string
// implementation; flaglint-go starts from the corrected, actual contract.
//
// Counts is the optional v1-additive multiset extension (spec/
// fingerprint.md in flaglint/spec): fingerprint -> occurrence count.
// Mitigates the v1 fingerprint's known static-collision limitation (two
// call sites sharing (callType, flagKey, file) share one fingerprint
// string), which otherwise lets a brand-new *duplicate* of an
// already-baselined call slip past --fail-on-new undetected — writers
// SHOULD emit it (Write always does); readers MUST accept a baseline
// without it, falling back to pure set semantics (Read/New both do).
type File struct {
	Version         string         `json:"version"`
	SchemaVersion   string         `json:"schemaVersion"`
	CreatedAt       string         `json:"createdAt"`
	FlaglintVersion string         `json:"flaglintVersion"`
	Fingerprints    []string       `json:"fingerprints"`
	Counts          map[string]int `json:"counts,omitempty"`
}

const currentVersion = "1"

// schemaVersion is the const value flaglint/spec's baseline.v1.schema.json
// expects for the "schemaVersion" field — a separate, additive field from
// "version" (the pre-existing "1" string), not a replacement for it.
const schemaVersion = "baseline.v1"

// Error carries the exit code a baseline failure should terminate the
// process with — always 2 (invalid input) per the exit-code contract;
// the type exists so callers can distinguish a baseline-specific failure
// from other *ExitError-shaped errors without a fragile message match.
type Error struct {
	Message string
}

func (e *Error) Error() string { return e.Message }

func errf(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

// Baseline is a parsed baseline file's contents, as needed for --fail-
// on-new comparison (New). Counts is nil when the file has no "counts"
// object at all (a baseline written before this feature, or a hand-
// crafted one) — New falls back to pure set semantics in that case,
// per spec.
type Baseline struct {
	Fingerprints map[string]bool
	Counts       map[string]int
}

// Read loads and validates a baseline file, returning its known
// fingerprint set and (if present) per-fingerprint counts. Returns
// *Error for a missing file, invalid JSON, wrong version, or a
// missing/malformed fingerprints array — the caller should treat this
// as exit code 2. An absent or malformed "counts" object is not an
// error: it's an optional, additive field, and its absence just means
// New falls back to set semantics.
func Read(path string) (Baseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Baseline{}, errf("baseline file not found: %s", path)
		}
		return Baseline{}, errf("failed to read baseline file %s: %v", path, err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return Baseline{}, errf("invalid JSON in baseline file: %s", path)
	}

	var version string
	if err := json.Unmarshal(obj["version"], &version); err != nil || version != currentVersion {
		return Baseline{}, errf(`unsupported baseline version in %s. Expected version "1"`, path)
	}

	// A literal JSON `null` for fingerprints must be rejected the same as a
	// missing or malformed array: json.Unmarshal("null", &fingerprints)
	// succeeds with err == nil (Go's null-into-slice semantics leave it
	// nil), which would otherwise silently treat a corrupt baseline as a
	// valid empty one — every current finding would then look "new" under
	// --fail-on-new instead of failing loudly with the expected exit 2.
	// The TS reference doesn't have this gap: Array.isArray(null) is false,
	// so its equivalent check rejects null for free.
	rawFingerprints, ok := obj["fingerprints"]
	if !ok || strings.TrimSpace(string(rawFingerprints)) == "null" {
		return Baseline{}, errf("baseline file is missing a valid 'fingerprints' array: %s", path)
	}
	var fingerprints []string
	if json.Unmarshal(rawFingerprints, &fingerprints) != nil {
		return Baseline{}, errf("baseline file is missing a valid 'fingerprints' array: %s", path)
	}

	set := make(map[string]bool, len(fingerprints))
	for _, fp := range fingerprints {
		set[fp] = true
	}

	// "counts" is optional and additive — a missing or explicitly-null
	// object is simply absent (nil), not an error; only a *present but
	// malformed* one (wrong JSON shape) is worth surfacing, since that
	// suggests real corruption rather than an older/hand-written file.
	var counts map[string]int
	if rawCounts, ok := obj["counts"]; ok && strings.TrimSpace(string(rawCounts)) != "null" {
		if err := json.Unmarshal(rawCounts, &counts); err != nil {
			return Baseline{}, errf("baseline file has a malformed 'counts' object: %s", path)
		}
	}

	return Baseline{Fingerprints: set, Counts: counts}, nil
}

// CountFingerprints returns how many times each fingerprint appears in
// fingerprints — the shared shape both Write (baselining today's counts)
// and a --fail-on-new caller (computing the current scan's own counts to
// compare against a baseline's) need.
func CountFingerprints(fingerprints []string) map[string]int {
	counts := make(map[string]int, len(fingerprints))
	for _, fp := range fingerprints {
		counts[fp]++
	}
	return counts
}

// Write deduplicates and sorts fingerprints and writes them to path,
// along with each fingerprint's occurrence count (see File.Counts),
// creating parent directories as needed.
func Write(path string, fingerprints []string, flaglintVersion string) error {
	counts := CountFingerprints(fingerprints)

	deduped := make([]string, 0, len(counts))
	for fp := range counts {
		deduped = append(deduped, fp)
	}
	sort.Strings(deduped)

	file := File{
		Version:         currentVersion,
		SchemaVersion:   schemaVersion,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		FlaglintVersion: flaglintVersion,
		Fingerprints:    deduped,
		Counts:          counts,
	}

	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return errf("failed to encode baseline: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errf("failed to write baseline file %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o644); err != nil {
		return errf("failed to write baseline file %s: %v", path, err)
	}
	return nil
}

// New returns every fingerprint that represents a genuinely new finding
// beyond known: either the fingerprint is entirely absent from known's
// set (existing, pre-counts set semantics), or — when known carries the
// optional counts extension — the current scan's occurrence count for
// that fingerprint exceeds the baselined one. The latter is what closes
// the "ratchet hole" a bare fingerprint set can't see on its own: a
// brand-new *duplicate* of an already-baselined call shares that call's
// fingerprint (the v1 format's known static-collision limitation — see
// spec/fingerprint.md) and would otherwise silently pass --fail-on-new.
//
// currentCounts is the current scan's own per-fingerprint occurrence
// count (CountFingerprints over every usage's fingerprint, unfiltered by
// known). A fingerprint present in known.Fingerprints but absent from
// known.Counts (only possible for a malformed/hand-edited baseline, since
// Write always emits a complete, consistent counts object) is treated as
// a baselined count of 0, per spec — the conservative, "don't silently
// pass debt through" fallback. When known.Counts is nil altogether (a
// baseline written before this feature, or a hand-crafted one), this
// falls back to pure set semantics — matching the spec's "readers MUST
// accept baselines without it" requirement.
func New(currentCounts map[string]int, known Baseline) []string {
	newFindings := []string{}
	for fp, count := range currentCounts {
		if !known.Fingerprints[fp] {
			newFindings = append(newFindings, fp)
			continue
		}
		if known.Counts == nil {
			continue // no counts extension in play — pure set semantics
		}
		if count > known.Counts[fp] { // absent from Counts -> 0, per spec
			newFindings = append(newFindings, fp)
		}
	}
	sort.Strings(newFindings)
	return newFindings
}
