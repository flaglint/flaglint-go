package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRead_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")

	if err := Write(path, []string{"b", "a", "b", "c"}, "1.0.0"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got.Fingerprints) != 3 || !got.Fingerprints["a"] || !got.Fingerprints["b"] || !got.Fingerprints["c"] {
		t.Errorf("Read().Fingerprints = %v, want {a,b,c}", got.Fingerprints)
	}
	want := map[string]int{"a": 1, "b": 2, "c": 1}
	if len(got.Counts) != len(want) {
		t.Fatalf("Read().Counts = %v, want %v", got.Counts, want)
	}
	for k, v := range want {
		if got.Counts[k] != v {
			t.Errorf("Read().Counts[%q] = %d, want %d", k, got.Counts[k], v)
		}
	}
}

func TestWrite_dedupedAndSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := Write(path, []string{"z", "a", "z", "m"}, "1.0.0"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "m", "z"}
	if len(f.Fingerprints) != len(want) {
		t.Fatalf("Fingerprints = %v, want %v", f.Fingerprints, want)
	}
	for i, w := range want {
		if f.Fingerprints[i] != w {
			t.Errorf("Fingerprints[%d] = %q, want %q", i, f.Fingerprints[i], w)
		}
	}
	if f.Version != "1" {
		t.Errorf("Version = %q, want string \"1\"", f.Version)
	}
	if f.SchemaVersion != "baseline.v1" {
		t.Errorf("SchemaVersion = %q, want baseline.v1 (flaglint/spec's baseline.v1.schema.json)", f.SchemaVersion)
	}
}

func TestWrite_versionIsJSONString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := Write(path, nil, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version": "1"`) {
		t.Errorf("baseline file must serialize version as the JSON string \"1\", got:\n%s", raw)
	}
}

func TestWrite_createsParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "baseline.json")
	if err := Write(path, []string{"a"}, "1.0.0"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("baseline file not created: %v", err)
	}
}

func TestWrite_emitsCounts(t *testing.T) {
	// A duplicate call (two occurrences sharing one v1 fingerprint) must
	// be recorded as count 2, not silently collapsed to 1 the way the
	// deduplicated fingerprints array itself is — this is the whole point
	// of the counts extension (spec/fingerprint.md's "ratchet hole" fix).
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := Write(path, []string{"dup", "dup", "single"}, "1.0.0"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Counts["dup"] != 2 {
		t.Errorf("Counts[dup] = %d, want 2", f.Counts["dup"])
	}
	if f.Counts["single"] != 1 {
		t.Errorf("Counts[single] = %d, want 1", f.Counts["single"])
	}
}

func TestRead_missingFile(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("Read() error = nil, want error for missing file")
	}
}

func TestRead_invalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("{not valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for invalid JSON")
	}
}

func TestRead_wrongVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": "2", "fingerprints": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for unsupported version")
	}
}

func TestRead_versionAsNumberRejected(t *testing.T) {
	// The cross-tool contract requires version to be the JSON string "1",
	// not the number 1 — a numeric version must be rejected, not coerced.
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": 1, "fingerprints": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for numeric version (must be string \"1\")")
	}
}

func TestRead_nullFingerprintsRejected(t *testing.T) {
	// A literal JSON null must be rejected the same as a missing/malformed
	// array — Go's json.Unmarshal("null", &slice) succeeds with a nil
	// result, which would otherwise silently accept this as a valid empty
	// baseline instead of failing loudly (exit 2).
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": "1", "fingerprints": null}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for null fingerprints")
	}
}

func TestRead_missingFingerprintsArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": "1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for missing fingerprints array")
	}
}

func TestRead_noCountsIsNilNotError(t *testing.T) {
	// A baseline written before this feature (or hand-crafted) has no
	// "counts" field at all — readers MUST accept it (spec), falling back
	// to pure set semantics rather than erroring.
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": "1", "fingerprints": ["a"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got.Counts != nil {
		t.Errorf("Counts = %v, want nil", got.Counts)
	}
	if !got.Fingerprints["a"] {
		t.Error("Fingerprints[a] = false, want true")
	}
}

func TestRead_malformedCountsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte(`{"version": "1", "fingerprints": ["a"], "counts": "not-an-object"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("Read() error = nil, want error for malformed counts object")
	}
}

func TestNew(t *testing.T) {
	known := Baseline{Fingerprints: map[string]bool{"a": true, "b": true}}
	got := New(CountFingerprints([]string{"a", "b", "c", "d"}), known)
	want := []string{"c", "d"}
	if len(got) != len(want) {
		t.Fatalf("New() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("New()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNew_noneNewReturnsEmptyNotNil(t *testing.T) {
	known := Baseline{Fingerprints: map[string]bool{"a": true}}
	got := New(CountFingerprints([]string{"a"}), known)
	if got == nil {
		t.Error("New() = nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("New() = %v, want empty", got)
	}
}

func TestNew_noCountsExtensionFallsBackToSetSemantics(t *testing.T) {
	// known.Counts is nil (baseline predates this feature) — a duplicate
	// occurrence of an already-known fingerprint must NOT be flagged as
	// new; that's exactly what the counts extension exists to change,
	// and its absence must mean the old, pre-feature behavior.
	known := Baseline{Fingerprints: map[string]bool{"dup": true}}
	got := New(CountFingerprints([]string{"dup", "dup"}), known)
	if len(got) != 0 {
		t.Errorf("New() = %v, want empty (no counts extension in play)", got)
	}
}

func TestNew_duplicateCallExceedingBaselinedCountIsNew(t *testing.T) {
	// The "ratchet hole" fix itself: a fingerprint already in the baseline
	// set, baselined at count 1, now appearing twice in the current scan —
	// a genuinely new duplicate call site — must be flagged as new.
	known := Baseline{
		Fingerprints: map[string]bool{"dup": true},
		Counts:       map[string]int{"dup": 1},
	}
	got := New(CountFingerprints([]string{"dup", "dup"}), known)
	if len(got) != 1 || got[0] != "dup" {
		t.Errorf("New() = %v, want [dup]", got)
	}
}

func TestNew_countNotExceedingBaselineIsNotNew(t *testing.T) {
	known := Baseline{
		Fingerprints: map[string]bool{"dup": true},
		Counts:       map[string]int{"dup": 2},
	}
	got := New(CountFingerprints([]string{"dup", "dup"}), known)
	if len(got) != 0 {
		t.Errorf("New() = %v, want empty (current count does not exceed baselined count)", got)
	}
}

func TestNew_fingerprintAbsentFromCountsTreatedAsZero(t *testing.T) {
	// A fingerprint present in the set but absent from counts (only
	// possible for a malformed/hand-edited baseline, since Write always
	// emits a complete, consistent counts object) must be treated as
	// baselined at count 0, per spec — the conservative fallback.
	known := Baseline{
		Fingerprints: map[string]bool{"a": true, "b": true},
		Counts:       map[string]int{"a": 1}, // "b" deliberately absent
	}
	got := New(CountFingerprints([]string{"a", "b"}), known)
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("New() = %v, want [b]", got)
	}
}
