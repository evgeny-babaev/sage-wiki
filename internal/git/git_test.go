package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAvailable(t *testing.T) {
	// Git should be available in CI/dev environments
	if !IsAvailable() {
		t.Skip("git not available")
	}
}

func TestInitAndIsRepo(t *testing.T) {
	if !IsAvailable() {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	if IsRepo(dir) {
		t.Error("should not be a repo yet")
	}

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !IsRepo(dir) {
		t.Error("should be a repo after init")
	}
}

func TestAutoCommit(t *testing.T) {
	if !IsAvailable() {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	Init(dir)

	// Configure git user for commit
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")

	// Create a file and commit
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	if err := AutoCommit(dir, "test commit"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	hash, msg, err := LastCommit(dir)
	if err != nil {
		t.Fatalf("LastCommit: %v", err)
	}
	if hash == "" {
		t.Error("expected commit hash")
	}
	if msg != "test commit" {
		t.Errorf("expected 'test commit', got %q", msg)
	}
}

func TestStatus(t *testing.T) {
	if !IsAvailable() {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	Init(dir)

	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0644)

	status, err := Status(dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status == "" {
		t.Error("expected non-empty status with untracked file")
	}
}

func TestPush(t *testing.T) {
	if !IsAvailable() {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "init", "--bare", remote)
	runGit(t, dir, "remote", "add", "origin", remote)

	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if err := AutoCommit(dir, "test push"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}
	branch, err := execGit("-C", dir, "branch", "--show-current")
	if err != nil {
		t.Fatalf("get current branch: %v", err)
	}
	runGit(t, dir, "push", "--set-upstream", "origin", strings.TrimSpace(branch))

	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("world"), 0644); err != nil {
		t.Fatalf("write second file: %v", err)
	}
	if err := AutoCommit(dir, "second push"); err != nil {
		t.Fatalf("AutoCommit second: %v", err)
	}
	if err := Push(dir); err != nil {
		t.Fatalf("Push: %v", err)
	}

	remoteHead, err := execGit("--git-dir", remote, "rev-parse", strings.TrimSpace(branch))
	if err != nil || strings.TrimSpace(remoteHead) == "" {
		t.Fatalf("remote branch was not pushed: %s: %v", remoteHead, err)
	}
}

func TestGitHubHTTPSRemote(t *testing.T) {
	if !IsAvailable() {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "git@github.com:owner/wiki.git")

	remote, err := githubHTTPSRemote(dir)
	if err != nil {
		t.Fatalf("githubHTTPSRemote: %v", err)
	}
	if remote != "https://github.com/owner/wiki.git" {
		t.Fatalf("unexpected remote: %s", remote)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := append([]string{"-C", dir}, args...)
	out, err := execGit(cmd...)
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}

func execGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
