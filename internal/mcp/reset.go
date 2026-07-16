package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/xoai/sage-wiki/internal/compiler"
	"github.com/xoai/sage-wiki/internal/config"
	"github.com/xoai/sage-wiki/internal/manifest"
)

type resetMove struct {
	original string
	backup   string
}

func (s *Server) handleResetKnowledge(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	cfg, err := config.Load(filepath.Join(s.projectDir, "config.yaml"))
	if err != nil {
		return errorResult(fmt.Sprintf("reset failed: load config: %v", err)), nil
	}
	confirmProject, _ := req.GetArguments()["confirm_project"].(string)
	if confirmProject != cfg.Project {
		return errorResult(fmt.Sprintf("confirm_project must exactly match %q", cfg.Project)), nil
	}

	var sources, concepts int
	acquired, err := s.coordinator.TryCompile(func() error {
		var resetErr error
		sources, concepts, resetErr = s.resetKnowledge(cfg)
		return resetErr
	})
	if !acquired {
		return errorResult("reset blocked: a compile is currently running"), nil
	}
	if err != nil {
		return errorResult(fmt.Sprintf("reset failed: %v", err)), nil
	}

	return textResult(fmt.Sprintf(
		"Knowledge reset complete:\n- Removed sources: %d\n- Removed concepts: %d\n- Preserved: config.yaml, purpose.md, index_intro.md\n- Regenerated: %s/index.md",
		sources, concepts, filepath.ToSlash(cfg.Output),
	)), nil
}

func (s *Server) resetKnowledge(cfg *config.Config) (int, int, error) {
	if cfg.IsVaultOverlay() {
		return 0, 0, fmt.Errorf("knowledge reset is disabled for vault-overlay projects")
	}

	mfPath := filepath.Join(s.projectDir, ".manifest.json")
	mf, err := manifest.Load(mfPath)
	if err != nil {
		return 0, 0, err
	}
	sourceCount, conceptCount := mf.SourceCount(), mf.ConceptCount()

	backupRoot := filepath.Join(s.projectDir, ".sage", fmt.Sprintf("reset-backup-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return 0, 0, fmt.Errorf("create reset backup: %w", err)
	}

	targets := append([]string{}, cfg.ResolveSources(s.projectDir)...)
	targets = append(targets, cfg.ResolveOutput(s.projectDir))
	targets, err = safeResetTargets(s.projectDir, targets)
	if err != nil {
		_ = os.RemoveAll(backupRoot)
		return 0, 0, err
	}

	var moves []resetMove
	rollback := func() {
		for i := len(moves) - 1; i >= 0; i-- {
			_ = os.RemoveAll(moves[i].original)
			_ = os.Rename(moves[i].backup, moves[i].original)
		}
		_ = os.RemoveAll(backupRoot)
	}

	for i, target := range targets {
		if _, statErr := os.Lstat(target); statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			rollback()
			return 0, 0, fmt.Errorf("inspect reset target %s: %w", target, statErr)
		}
		backup := filepath.Join(backupRoot, fmt.Sprintf("tree-%d", i))
		if err := os.Rename(target, backup); err != nil {
			rollback()
			return 0, 0, fmt.Errorf("backup reset target %s: %w", target, err)
		}
		moves = append(moves, resetMove{original: target, backup: backup})
	}

	for _, file := range []string{
		mfPath,
		filepath.Join(s.projectDir, ".sage", "compile-state.json"),
		filepath.Join(s.projectDir, ".sage", "purpose-recompile-backup"),
	} {
		if _, statErr := os.Stat(file); statErr == nil {
			backup := filepath.Join(backupRoot, fmt.Sprintf("file-%d", len(moves)))
			if err := os.Rename(file, backup); err != nil {
				rollback()
				return 0, 0, fmt.Errorf("backup reset file %s: %w", file, err)
			}
			moves = append(moves, resetMove{original: file, backup: backup})
		} else if !os.IsNotExist(statErr) {
			rollback()
			return 0, 0, fmt.Errorf("inspect reset file %s: %w", file, statErr)
		}
	}

	empty := manifest.New()
	if err := saveManifestAtomic(mfPath, empty); err != nil {
		rollback()
		return 0, 0, fmt.Errorf("write empty manifest: %w", err)
	}
	purpose, err := compiler.LoadPurpose(s.projectDir)
	if err != nil {
		rollback()
		return 0, 0, err
	}
	if err := compiler.GenerateWikiIndex(s.projectDir, cfg, empty, purpose); err != nil {
		rollback()
		return 0, 0, err
	}
	for _, sourceDir := range cfg.ResolveSources(s.projectDir) {
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			rollback()
			return 0, 0, fmt.Errorf("recreate source directory: %w", err)
		}
	}

	if err := s.db.ResetKnowledge(); err != nil {
		rollback()
		return 0, 0, fmt.Errorf("clear knowledge database: %w", err)
	}
	if err := os.RemoveAll(backupRoot); err != nil {
		return 0, 0, fmt.Errorf("remove reset backup: %w", err)
	}
	s.cfg = cfg
	return sourceCount, conceptCount, nil
}

func saveManifestAtomic(path string, mf *manifest.Manifest) error {
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".manifest-reset-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func safeResetTargets(projectDir string, targets []string) ([]string, error) {
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var safe []string
	for _, target := range targets {
		abs, err := filepath.Abs(target)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, err
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("unsafe reset target outside project: %s", target)
		}
		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		if first == ".git" || first == ".sage" {
			return nil, fmt.Errorf("unsafe reset target: %s", target)
		}
		key := strings.ToLower(filepath.Clean(abs))
		if !seen[key] {
			seen[key] = true
			safe = append(safe, abs)
		}
	}
	return safe, nil
}
