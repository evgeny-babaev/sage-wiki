package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateModelStagesPreservesEnvironmentPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := `version: 1
project: test
sources:
  - path: raw
    type: auto
output: wiki
api:
  provider: openai-compatible
  api_key: ${SECRET_API_KEY}
models:
  summarize: existing-summary
  extract: existing-extract
  write: existing-write
  lint: existing-lint
  query: existing-query
compiler: {}
search: {}
serve: {}
`
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	err := UpdateModelStages(path, map[string]ModelStageUpdate{
		"extract": {Model: "openai/gpt-5.6-luna", ExtraParams: map[string]string{"reasoning_effort": "medium"}},
		"write":   {Model: "openai/gpt-5.6-luna", ExtraParams: map[string]string{"reasoning_effort": "low"}},
	})
	if err != nil {
		t.Fatalf("UpdateModelStages: %v", err)
	}

	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "api_key: ${SECRET_API_KEY}") {
		t.Fatalf("environment placeholder was expanded or removed:\n%s", stored)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Models.Summarize != "existing-summary" || cfg.Models.Lint != "existing-lint" || cfg.Models.Query != "existing-query" {
		t.Fatalf("unrelated models changed: %+v", cfg.Models)
	}
	if cfg.Models.Extract != "openai/gpt-5.6-luna" || cfg.Models.Write != "openai/gpt-5.6-luna" {
		t.Fatalf("compile models not updated: %+v", cfg.Models)
	}
	if got := cfg.Models.ParamsFor("extract")["reasoning_effort"]; got != "medium" {
		t.Fatalf("extract reasoning_effort = %v", got)
	}
	if got := cfg.Models.ParamsFor("write")["reasoning_effort"]; got != "low" {
		t.Fatalf("write reasoning_effort = %v", got)
	}
}
