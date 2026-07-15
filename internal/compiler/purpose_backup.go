package compiler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const purposeBackupDir = "purpose-recompile-backup"

type purposeBackupMetadata struct {
	Output string `json:"output"`
}

type purposeRecompileBackup struct {
	projectDir string
	dir        string
	output     string
	done       bool
}

func recoverPendingPurposeBackup(projectDir string) error {
	dir := filepath.Join(projectDir, ".sage", purposeBackupDir)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect purpose backup: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "committed")); err == nil {
		return os.RemoveAll(dir)
	}

	backup, err := loadPurposeBackup(projectDir, dir)
	if err != nil {
		return err
	}
	return backup.Rollback()
}

func createPurposeBackup(projectDir, output string) (*purposeRecompileBackup, error) {
	dir := filepath.Join(projectDir, ".sage", purposeBackupDir)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("clear purpose backup: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create purpose backup: %w", err)
	}

	metadata, err := json.Marshal(purposeBackupMetadata{Output: output})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0644); err != nil {
		return nil, fmt.Errorf("write purpose backup metadata: %w", err)
	}

	backup := &purposeRecompileBackup{projectDir: projectDir, dir: dir, output: output}
	artifacts := []struct {
		source string
		name   string
	}{
		{filepath.Join(projectDir, ".manifest.json"), "manifest.json"},
		{filepath.Join(projectDir, ".sage", "wiki.db"), "wiki.db"},
		{filepath.Join(projectDir, ".sage", "wiki.db-wal"), "wiki.db-wal"},
		{filepath.Join(projectDir, ".sage", "wiki.db-shm"), "wiki.db-shm"},
		{filepath.Join(projectDir, output, "index.md"), "index.md"},
	}
	for _, artifact := range artifacts {
		if err := copyFileIfExists(artifact.source, filepath.Join(dir, artifact.name)); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
	}
	for _, subdir := range []string{"summaries", "concepts"} {
		if err := copyDirIfExists(filepath.Join(projectDir, output, subdir), filepath.Join(dir, subdir)); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
	}
	return backup, nil
}

func loadPurposeBackup(projectDir, dir string) (*purposeRecompileBackup, error) {
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("read purpose backup metadata: %w", err)
	}
	var metadata purposeBackupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("parse purpose backup metadata: %w", err)
	}
	return &purposeRecompileBackup{projectDir: projectDir, dir: dir, output: metadata.Output}, nil
}

func (b *purposeRecompileBackup) Commit() error {
	if b == nil || b.done {
		return nil
	}
	if err := os.WriteFile(filepath.Join(b.dir, "committed"), []byte("ok\n"), 0644); err != nil {
		return fmt.Errorf("mark purpose backup committed: %w", err)
	}
	b.done = true
	if err := os.RemoveAll(b.dir); err != nil {
		return fmt.Errorf("remove purpose backup: %w", err)
	}
	return nil
}

func (b *purposeRecompileBackup) Rollback() error {
	if b == nil || b.done {
		return nil
	}
	files := []struct {
		backup string
		target string
	}{
		{filepath.Join(b.dir, "manifest.json"), filepath.Join(b.projectDir, ".manifest.json")},
		{filepath.Join(b.dir, "wiki.db"), filepath.Join(b.projectDir, ".sage", "wiki.db")},
		{filepath.Join(b.dir, "wiki.db-wal"), filepath.Join(b.projectDir, ".sage", "wiki.db-wal")},
		{filepath.Join(b.dir, "wiki.db-shm"), filepath.Join(b.projectDir, ".sage", "wiki.db-shm")},
		{filepath.Join(b.dir, "index.md"), filepath.Join(b.projectDir, b.output, "index.md")},
	}
	for _, file := range files {
		if err := restoreOptionalFile(file.backup, file.target); err != nil {
			return err
		}
	}
	for _, subdir := range []string{"summaries", "concepts"} {
		target := filepath.Join(b.projectDir, b.output, subdir)
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := copyDirIfExists(filepath.Join(b.dir, subdir), target); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(b.dir); err != nil {
		return err
	}
	b.done = true
	return nil
}

func restoreOptionalFile(backup, target string) error {
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return copyFileIfExists(backup, target)
}

func copyFileIfExists(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return nil
}

func copyDirIfExists(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("expected directory: %s", source)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0755)
		}
		return copyFileIfExists(path, destination)
	})
}
