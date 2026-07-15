package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xoai/sage-wiki/internal/config"
)

func TestScanSnapshotTracksPurposeOutsideSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0755); err != nil {
		t.Fatal(err)
	}
	sources := []config.Source{{Path: "raw"}}
	before := scanSnapshot(dir, sources, nil)
	purposePath := filepath.Join(dir, PurposeFilename)
	if before[purposePath] != "missing" {
		t.Fatalf("missing purpose snapshot = %q", before[purposePath])
	}
	if err := os.WriteFile(purposePath, []byte("purpose"), 0644); err != nil {
		t.Fatal(err)
	}
	after := scanSnapshot(dir, sources, nil)
	if after[purposePath] == "" || after[purposePath] == before[purposePath] {
		t.Fatalf("purpose change not reflected: before=%q after=%q", before[purposePath], after[purposePath])
	}
}
