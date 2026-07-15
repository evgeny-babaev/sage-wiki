package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xoai/sage-wiki/internal/log"
)

// IsAvailable checks if git is installed and accessible.
func IsAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo checks if the given directory is inside a git repository.
func IsRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// Init initializes a new git repository in the given directory.
func Init(dir string) error {
	if !IsAvailable() {
		log.Warn("git not available, skipping init")
		return nil
	}
	cmd := exec.Command("git", "-C", dir, "init")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git.Init: %s: %w", string(out), err)
	}
	log.Info("git repository initialized", "dir", dir)
	return nil
}

// Add stages files for commit.
func Add(dir string, paths ...string) error {
	if !IsAvailable() {
		return nil
	}
	args := append([]string{"-C", dir, "add"}, paths...)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git.Add: %s: %w", string(out), err)
	}
	return nil
}

// Commit creates a commit with the given message.
func Commit(dir string, message string) error {
	if !IsAvailable() {
		return nil
	}
	cmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// "nothing to commit" is not an error
		if strings.Contains(string(out), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git.Commit: %s: %w", string(out), err)
	}
	log.Info("git commit created", "message", message)
	return nil
}

// AutoCommit stages all changes and commits with a structured message.
func AutoCommit(dir string, message string) error {
	if !IsAvailable() || !IsRepo(dir) {
		return nil
	}
	if err := Add(dir, "."); err != nil {
		return err
	}
	return Commit(dir, message)
}

// Push publishes the current branch to its configured upstream.
func Push(dir string) error {
	if !IsAvailable() || !IsRepo(dir) {
		return nil
	}
	cmd := exec.Command("git", "-C", dir, "push")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git.Push: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Status returns the short status output.
func Status(dir string) (string, error) {
	if !IsAvailable() {
		return "", nil
	}
	cmd := exec.Command("git", "-C", dir, "status", "--short")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git.Status: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// LastCommit returns the last commit hash and message.
func LastCommit(dir string) (hash string, message string, err error) {
	if !IsAvailable() || !IsRepo(dir) {
		return "", "", nil
	}
	cmd := exec.Command("git", "-C", dir, "log", "-1", "--format=%h %s")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", nil // no commits yet is not an error
	}
	line := strings.TrimSpace(string(out))
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return line, "", nil
}

// DetectRenames uses git status to detect renamed files.
// Returns a map of old_path -> new_path.
func DetectRenames(dir string) (map[string]string, error) {
	if !IsAvailable() || !IsRepo(dir) {
		return nil, nil
	}

	cmd := exec.Command("git", "-C", dir, "status", "--porcelain=v2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, nil
	}

	renames := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		// Porcelain v2 rename format: "2 R. ..."
		if strings.HasPrefix(line, "2 R") {
			parts := strings.Fields(line)
			if len(parts) >= 10 {
				oldPath := parts[9]
				newPath := parts[8]
				abs := func(p string) string {
					if filepath.IsAbs(p) {
						return p
					}
					return filepath.Join(dir, p)
				}
				renames[abs(oldPath)] = abs(newPath)
			}
		}
	}

	return renames, nil
}
