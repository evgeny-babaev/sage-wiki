package compiler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xoai/sage-wiki/internal/manifest"
	"github.com/xoai/sage-wiki/internal/wiki"
)

func TestCompilePurposeChangeRebuildsDerivedKnowledge(t *testing.T) {
	const purposeText = "Support project delivery decisions."
	const failingPurpose = "Force purpose rebuild failure."
	var mu sync.Mutex
	var purposePrompts []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		messages, _ := body["messages"].([]any)
		last := ""
		if len(messages) > 0 {
			if msg, ok := messages[len(messages)-1].(map[string]any); ok {
				last, _ = msg["content"].(string)
			}
		}
		hasPurpose := strings.Contains(last, purposeText)
		if hasPurpose {
			mu.Lock()
			purposePrompts = append(purposePrompts, last)
			mu.Unlock()
		}

		content := "## Key claims\n\nThis is a sufficiently long summary of the source material for the purpose-aware compilation integration test.\n\n## Concepts\n\nA project concept."
		switch {
		case strings.Contains(last, "concept extraction system"):
			if strings.Contains(last, failingPurpose) {
				content = "not valid json"
				break
			}
			name := "old-concept"
			if hasPurpose {
				name = "new-concept"
			}
			content = `[{"name":"` + name + `","aliases":[],"sources":["raw/article.md"],"type":"concept"}]`
		case strings.Contains(last, "wiki author writing a comprehensive article"):
			name := "old-concept"
			if hasPurpose {
				name = "new-concept"
			}
			content = "# " + name + "\n\n## Definition\n\nA grounded project concept."
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
			"model":   "gpt-4o-mini",
			"usage":   map[string]int{"total_tokens": 100},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	if err := wiki.InitGreenfield(dir, "test", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	cfg := `
version: 1
project: test
sources:
  - path: raw
    type: article
    watch: true
output: wiki
api:
  provider: openai
  api_key: sk-test
  base_url: ` + server.URL + `
models:
  summarize: gpt-4o-mini
  extract: gpt-4o-mini
  write: gpt-4o-mini
compiler:
  max_parallel: 1
  auto_commit: false
  summary_max_tokens: 500
  article_max_tokens: 500
  default_tier: 3
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw", "article.md"), []byte("# Project\n\nStable source content."), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := Compile(dir, CompileOpts{})
	if err != nil {
		t.Fatalf("first Compile: %v", err)
	}
	if first.ArticlesWritten != 1 {
		t.Fatalf("first compile wrote %d articles", first.ArticlesWritten)
	}
	oldPath := filepath.Join(dir, "wiki", "concepts", "old-concept.md")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old concept missing after first compile: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte("# Purpose\n\n"+purposeText+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := Compile(dir, CompileOpts{})
	if err != nil {
		t.Fatalf("second Compile: %v", err)
	}
	if second.Modified != 1 || second.Summarized != 1 || second.ArticlesWritten != 1 {
		t.Fatalf("purpose change did not trigger full rebuild: %+v", second)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old concept should be removed after purpose change, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "wiki", "concepts", "new-concept.md")); err != nil {
		t.Fatalf("new concept missing: %v", err)
	}

	mf, err := manifest.Load(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	purpose, err := LoadPurpose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mf.PurposeHash == "" || mf.PurposeHash != purpose.Hash {
		t.Fatalf("manifest purpose hash not updated: manifest=%q purpose=%q", mf.PurposeHash, purpose.Hash)
	}
	if _, ok := mf.Concepts["old-concept"]; ok {
		t.Fatal("old concept remained in manifest")
	}

	mu.Lock()
	prompts := append([]string(nil), purposePrompts...)
	mu.Unlock()
	if len(prompts) < 3 {
		t.Fatalf("purpose was not passed to all three LLM passes; captured %d prompts", len(prompts))
	}
	var sawExtract, sawWrite bool
	for _, prompt := range prompts {
		sawExtract = sawExtract || strings.Contains(prompt, "concept extraction system")
		sawWrite = sawWrite || strings.Contains(prompt, "wiki author writing a comprehensive article")
	}
	if !sawExtract || !sawWrite {
		t.Fatalf("purpose prompts missing extract/write: extract=%v write=%v", sawExtract, sawWrite)
	}

	stableHash := mf.PurposeHash
	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte(failingPurpose+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	failed, err := Compile(dir, CompileOpts{})
	if err != nil {
		t.Fatalf("failed purpose rebuild returned fatal error: %v", err)
	}
	if failed.Errors == 0 {
		t.Fatal("expected purpose rebuild to report an extraction error")
	}
	restored, err := manifest.Load(filepath.Join(dir, ".manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if restored.PurposeHash != stableHash {
		t.Fatalf("failed rebuild changed purpose hash: got %q want %q", restored.PurposeHash, stableHash)
	}
	if _, err := os.Stat(filepath.Join(dir, "wiki", "concepts", "new-concept.md")); err != nil {
		t.Fatalf("failed rebuild did not restore prior concept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sage", purposeBackupDir)); !os.IsNotExist(err) {
		t.Fatalf("purpose backup was not cleaned after rollback: %v", err)
	}
}

func TestCompileNothingToDoGeneratesWikiIndex(t *testing.T) {
	dir := t.TempDir()
	if err := wiki.InitGreenfield(dir, "test", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(dir, CompileOpts{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "wiki", "index.md"))
	if err != nil {
		t.Fatalf("no-op compile did not generate index: %v", err)
	}
	if !strings.Contains(string(data), "# test") || strings.Contains(string(data), "## Purpose") {
		t.Fatalf("unexpected no-op index:\n%s", data)
	}
}
