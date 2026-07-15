package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPurposeBackupRollbackRestoresDerivedState(t *testing.T) {
	dir := t.TempDir()
	writeBackupFixture(t, filepath.Join(dir, ".manifest.json"), "old manifest")
	writeBackupFixture(t, filepath.Join(dir, ".sage", "wiki.db"), "old db")
	writeBackupFixture(t, filepath.Join(dir, "wiki", "index.md"), "old index")
	writeBackupFixture(t, filepath.Join(dir, "wiki", "summaries", "a.md"), "old summary")
	writeBackupFixture(t, filepath.Join(dir, "wiki", "concepts", "a.md"), "old concept")

	backup, err := createPurposeBackup(dir, "wiki")
	if err != nil {
		t.Fatalf("createPurposeBackup: %v", err)
	}
	writeBackupFixture(t, filepath.Join(dir, ".manifest.json"), "new manifest")
	writeBackupFixture(t, filepath.Join(dir, ".sage", "wiki.db"), "new db")
	writeBackupFixture(t, filepath.Join(dir, "wiki", "index.md"), "new index")
	if err := os.RemoveAll(filepath.Join(dir, "wiki", "summaries")); err != nil {
		t.Fatal(err)
	}
	writeBackupFixture(t, filepath.Join(dir, "wiki", "concepts", "new.md"), "new concept")

	if err := backup.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	assertBackupFixture(t, filepath.Join(dir, ".manifest.json"), "old manifest")
	assertBackupFixture(t, filepath.Join(dir, ".sage", "wiki.db"), "old db")
	assertBackupFixture(t, filepath.Join(dir, "wiki", "index.md"), "old index")
	assertBackupFixture(t, filepath.Join(dir, "wiki", "summaries", "a.md"), "old summary")
	assertBackupFixture(t, filepath.Join(dir, "wiki", "concepts", "a.md"), "old concept")
	if _, err := os.Stat(filepath.Join(dir, "wiki", "concepts", "new.md")); !os.IsNotExist(err) {
		t.Fatalf("new concept survived rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sage", purposeBackupDir)); !os.IsNotExist(err) {
		t.Fatalf("backup directory survived rollback: %v", err)
	}
}

func writeBackupFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertBackupFixture(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
