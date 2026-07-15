package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPurposeHashRoundTripAndLegacyCompatibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".manifest.json")
	m := New()
	m.PurposeHash = "sha256:test"
	if err := m.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PurposeHash != m.PurposeHash {
		t.Fatalf("purpose hash not preserved: %q", loaded.PurposeHash)
	}

	legacy := []byte(`{"version":2,"sources":{},"concepts":{}}`)
	if err := os.WriteFile(path, legacy, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil {
		t.Fatalf("Load legacy manifest: %v", err)
	}
	if loaded.PurposeHash != "" {
		t.Fatalf("legacy manifest should have empty purpose hash, got %q", loaded.PurposeHash)
	}
}
