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

	set, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(set) != 3 || !set["a"] || !set["b"] || !set["c"] {
		t.Errorf("Read() = %v, want {a,b,c}", set)
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

func TestNew(t *testing.T) {
	known := map[string]bool{"a": true, "b": true}
	got := New([]string{"a", "b", "c", "d"}, known)
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
	got := New([]string{"a"}, map[string]bool{"a": true})
	if got == nil {
		t.Error("New() = nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("New() = %v, want empty", got)
	}
}
