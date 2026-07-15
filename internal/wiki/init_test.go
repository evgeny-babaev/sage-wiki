package wiki

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGreenfield(t *testing.T) {
	dir := t.TempDir()

	if err := InitGreenfield(dir, "test-wiki", "gemini-2.5-flash"); err != nil {
		t.Fatalf("InitGreenfield: %v", err)
	}

	// Verify directory structure (no connections/ — see #91)
	expectedDirs := []string{
		"raw",
		"wiki/summaries",
		"wiki/concepts",
		"wiki/outputs",
		"wiki/images",
		"wiki/archive",
		".sage",
	}
	for _, d := range expectedDirs {
		path := filepath.Join(dir, d)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", d)
		}
	}

	// Verify config.yaml
	cfgPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config.yaml should exist")
	}

	// purpose.md is optional at runtime, but new projects get a comment-only
	// template that keeps purpose-aware compilation disabled until edited.
	purposePath := filepath.Join(dir, "purpose.md")
	purpose, err := os.ReadFile(purposePath)
	if err != nil {
		t.Fatalf("read purpose.md: %v", err)
	}
	if !strings.Contains(string(purpose), "wiki should help") {
		t.Errorf("purpose.md missing guidance: %q", string(purpose))
	}

	// Verify .gitignore
	gitignore := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != ".sage/\n" {
		t.Errorf("unexpected .gitignore content: %q", string(data))
	}

	// Verify manifest
	manifestPath := filepath.Join(dir, ".manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Error(".manifest.json should exist")
	}

	// Verify DB
	dbPath := filepath.Join(dir, ".sage", "wiki.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("wiki.db should exist")
	}
}

func TestInitVaultOverlay(t *testing.T) {
	dir := t.TempDir()

	// Create some vault folders
	os.MkdirAll(filepath.Join(dir, "Clippings"), 0755)
	os.MkdirAll(filepath.Join(dir, "Papers"), 0755)
	os.WriteFile(filepath.Join(dir, "Clippings", "test.md"), []byte("# Test"), 0644)

	err := InitVaultOverlay(dir, "my-vault",
		[]string{"Clippings", "Papers"},
		[]string{"Personal", "Daily Notes"},
		"_wiki",
		"gemini-2.5-flash",
	)
	if err != nil {
		t.Fatalf("InitVaultOverlay: %v", err)
	}

	// Verify _wiki structure (no connections/ — see #91)
	expectedDirs := []string{
		"_wiki/summaries",
		"_wiki/concepts",
		"_wiki/outputs",
		"_wiki/images",
		"_wiki/archive",
		".sage",
	}
	for _, d := range expectedDirs {
		path := filepath.Join(dir, d)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", d)
		}
	}

	// Source folders should NOT be modified
	clippingsTest := filepath.Join(dir, "Clippings", "test.md")
	if _, err := os.Stat(clippingsTest); os.IsNotExist(err) {
		t.Error("source files should not be modified")
	}
}

func TestScanFolders(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "Clippings"), 0755)
	os.MkdirAll(filepath.Join(dir, "Papers"), 0755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.MkdirAll(filepath.Join(dir, "Empty"), 0755)

	os.WriteFile(filepath.Join(dir, "Clippings", "a.md"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "Clippings", "b.md"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "Papers", "paper.pdf"), []byte("x"), 0644)

	folders, err := ScanFolders(dir)
	if err != nil {
		t.Fatalf("ScanFolders: %v", err)
	}

	// Should not include .hidden
	for _, f := range folders {
		if f.Name == ".hidden" {
			t.Error("should not include hidden folders")
		}
	}

	// Find Clippings
	var clippings *FolderInfo
	for i := range folders {
		if folders[i].Name == "Clippings" {
			clippings = &folders[i]
		}
	}
	if clippings == nil {
		t.Fatal("expected Clippings folder")
	}
	if clippings.FileCount != 2 {
		t.Errorf("expected 2 files in Clippings, got %d", clippings.FileCount)
	}
	if !clippings.HasMD {
		t.Error("Clippings should have .md files")
	}
}

// TestInitGreenfield_PreservesExistingConfig verifies that re-running
// `sage-wiki init` (e.g., to recover after deleting .sage/) does not
// overwrite a user's existing config.yaml. Fixes #84 obs 2.
func TestInitGreenfield_PreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	customConfig := "# user-customized config\nproject: my-existing\nversion: 1\n"
	if err := os.WriteFile(cfgPath, []byte(customConfig), 0644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := InitGreenfield(dir, "fresh-name", "gemini-2.5-flash"); err != nil {
		t.Fatalf("InitGreenfield: %v", err)
	}

	got, _ := os.ReadFile(cfgPath)
	if string(got) != customConfig {
		t.Errorf("config.yaml was overwritten\nwant:\n%s\ngot:\n%s", customConfig, string(got))
	}

	// Recovery scenario: .sage/ should still be created
	if _, err := os.Stat(filepath.Join(dir, ".sage")); err != nil {
		t.Error(".sage/ should be created even when config exists")
	}
}

func TestInitVaultOverlay_PreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	customConfig := "# vault user config\nproject: my-vault\n"
	if err := os.WriteFile(cfgPath, []byte(customConfig), 0644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := InitVaultOverlay(dir, "fresh-name", []string{"Notes"}, nil, "_wiki", "gemini-2.5-flash"); err != nil {
		t.Fatalf("InitVaultOverlay: %v", err)
	}

	got, _ := os.ReadFile(cfgPath)
	if string(got) != customConfig {
		t.Errorf("config.yaml was overwritten\nwant:\n%s\ngot:\n%s", customConfig, string(got))
	}
}

func TestInitGreenfield_WritesConfigWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	if err := InitGreenfield(dir, "new-project", "gemini-2.5-flash"); err != nil {
		t.Fatalf("InitGreenfield: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}
	if !strings.Contains(string(got), "project: new-project") {
		t.Errorf("config.yaml missing expected project name; got:\n%s", string(got))
	}
}

// TestInitGreenfield_DoesNotCreateConnectionsDir verifies that
// `sage-wiki init` no longer scaffolds an empty wiki/connections/ directory.
// Connection edges live in the SQLite relations table and surface via the
// ontology query/graph paths — an empty dir confused users (#91).
func TestInitGreenfield_DoesNotCreateConnectionsDir(t *testing.T) {
	dir := t.TempDir()
	if err := InitGreenfield(dir, "test", "gemini-2.5-flash"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wiki", "connections")); !os.IsNotExist(err) {
		t.Errorf("wiki/connections/ should NOT be created; stat err = %v", err)
	}
	// Sibling dirs should still exist
	for _, sub := range []string{"summaries", "concepts", "outputs", "images", "archive"} {
		if _, err := os.Stat(filepath.Join(dir, "wiki", sub)); err != nil {
			t.Errorf("wiki/%s/ should exist: %v", sub, err)
		}
	}
}

func TestInitVaultOverlay_DoesNotCreateConnectionsDir(t *testing.T) {
	dir := t.TempDir()
	if err := InitVaultOverlay(dir, "test-vault", []string{"Notes"}, nil, "_wiki", "gemini-2.5-flash"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_wiki", "connections")); !os.IsNotExist(err) {
		t.Errorf("_wiki/connections/ should NOT be created; stat err = %v", err)
	}
	for _, sub := range []string{"summaries", "concepts", "outputs", "images", "archive"} {
		if _, err := os.Stat(filepath.Join(dir, "_wiki", sub)); err != nil {
			t.Errorf("_wiki/%s/ should exist: %v", sub, err)
		}
	}
}
