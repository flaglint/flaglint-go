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
	"time"
)

// File is the on-disk baseline shape. Version is deliberately the string
// "1", not the number 1 — flaglint-js's own ADR 008 originally documented
// this as a number and had to be corrected to match the shipped string
// implementation; flaglint-go starts from the corrected, actual contract.
type File struct {
	Version         string   `json:"version"`
	CreatedAt       string   `json:"createdAt"`
	FlaglintVersion string   `json:"flaglintVersion"`
	Fingerprints    []string `json:"fingerprints"`
}

const currentVersion = "1"

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

// Read loads and validates a baseline file, returning the set of known
// fingerprints. Returns *Error for a missing file, invalid JSON, wrong
// version, or a missing/malformed fingerprints array — the caller should
// treat this as exit code 2.
func Read(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errf("baseline file not found: %s", path)
		}
		return nil, errf("failed to read baseline file %s: %v", path, err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errf("invalid JSON in baseline file: %s", path)
	}

	var version string
	if err := json.Unmarshal(obj["version"], &version); err != nil || version != currentVersion {
		return nil, errf(`unsupported baseline version in %s. Expected version "1"`, path)
	}

	var fingerprints []string
	if raw, ok := obj["fingerprints"]; !ok || json.Unmarshal(raw, &fingerprints) != nil {
		return nil, errf("baseline file is missing a valid 'fingerprints' array: %s", path)
	}

	set := make(map[string]bool, len(fingerprints))
	for _, fp := range fingerprints {
		set[fp] = true
	}
	return set, nil
}

// Write deduplicates and sorts fingerprints and writes them to path,
// creating parent directories as needed.
func Write(path string, fingerprints []string, flaglintVersion string) error {
	seen := map[string]bool{}
	deduped := make([]string, 0, len(fingerprints))
	for _, fp := range fingerprints {
		if !seen[fp] {
			seen[fp] = true
			deduped = append(deduped, fp)
		}
	}
	sort.Strings(deduped)

	file := File{
		Version:         currentVersion,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		FlaglintVersion: flaglintVersion,
		Fingerprints:    deduped,
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

// New returns the fingerprints in current that are not present in known.
func New(current []string, known map[string]bool) []string {
	newFindings := []string{}
	for _, fp := range current {
		if !known[fp] {
			newFindings = append(newFindings, fp)
		}
	}
	return newFindings
}
