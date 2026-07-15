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

// PushWithGitHubToken publishes the current branch without persisting the token.
// It is intended for isolated runtimes where the repository's SSH credential is
// deliberately not mounted into the process container.
func PushWithGitHubToken(dir, token string) error {
	if token == "" {
		return Push(dir)
	}
	if !IsAvailable() || !IsRepo(dir) {
		return nil
	}
	remote, err := githubHTTPSRemote(dir)
	if err != nil {
		return err
	}
	branchOut, err := exec.Command("git", "-C", dir, "branch", "--show-current").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git.Push: resolve current branch: %s: %w", strings.TrimSpace(string(branchOut)), err)
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" {
		return fmt.Errorf("git.Push: current HEAD is detached")
	}

	helper := `!f() { if [ "$1" = get ]; then printf '%s\n' 'username=x-access-token' "password=$SAGE_WIKI_GIT_TOKEN"; fi; }; f`
	cmd := exec.Command("git", "-c", "credential.helper="+helper, "-C", dir, "push", remote, "HEAD:refs/heads/"+branch)
	cmd.Env = append(cmd.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"SAGE_WIKI_GIT_TOKEN="+token,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git.Push: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func githubHTTPSRemote(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "--push", "origin").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git.Push: resolve origin: %s: %w", strings.TrimSpace(string(out)), err)
	}
	remote := strings.TrimSpace(string(out))
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		return "https://github.com/" + strings.TrimPrefix(remote, "git@github.com:"), nil
	case strings.HasPrefix(remote, "ssh://git@github.com/"):
		return "https://github.com/" + strings.TrimPrefix(remote, "ssh://git@github.com/"), nil
	case strings.HasPrefix(remote, "https://github.com/"):
		return remote, nil
	default:
		return "", fmt.Errorf("git.Push: token authentication supports github.com origins only")
	}
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
