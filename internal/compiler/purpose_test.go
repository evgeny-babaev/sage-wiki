package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xoai/sage-wiki/internal/config"
	"github.com/xoai/sage-wiki/internal/manifest"
	"github.com/xoai/sage-wiki/internal/storage"
)

func TestLoadPurposeOptionalAndStable(t *testing.T) {
	dir := t.TempDir()

	missing, err := LoadPurpose(dir)
	if err != nil {
		t.Fatalf("LoadPurpose missing: %v", err)
	}
	if missing.Enabled() || missing.Hash != "" {
		t.Fatalf("missing purpose should be disabled: %+v", missing)
	}

	commentOnly := "<!-- Describe the wiki purpose here. -->\n"
	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte(commentOnly), 0644); err != nil {
		t.Fatal(err)
	}
	placeholder, err := LoadPurpose(dir)
	if err != nil {
		t.Fatalf("LoadPurpose placeholder: %v", err)
	}
	if placeholder.Enabled() || placeholder.Hash != "" {
		t.Fatalf("comment-only purpose should be disabled: %+v", placeholder)
	}

	content := "# Purpose\n\nSupport product decisions.\n"
	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := LoadPurpose(dir)
	if err != nil {
		t.Fatalf("LoadPurpose content: %v", err)
	}
	if !first.Enabled() || first.Text != "# Purpose\n\nSupport product decisions." || first.Hash == "" {
		t.Fatalf("unexpected purpose: %+v", first)
	}

	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte("\n"+content+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := LoadPurpose(dir)
	if err != nil {
		t.Fatalf("LoadPurpose normalized content: %v", err)
	}
	if second.Hash != first.Hash || second.Text != first.Text {
		t.Fatalf("outer whitespace changed effective purpose: first=%+v second=%+v", first, second)
	}
}

func TestResetLLMPassesPreservesRawIndexState(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "wiki.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewCompileItemStore(db)
	if err := store.Upsert(CompileItem{
		SourcePath:     "raw/a.md",
		Hash:           "sha256:a",
		Tier:           3,
		PassIndexed:    true,
		PassEmbedded:   true,
		PassParsed:     true,
		PassSummarized: true,
		PassExtracted:  true,
		PassWritten:    true,
		SummaryPath:    "wiki/summaries/a.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetLLMPasses([]string{"raw/a.md"}); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetByPath("raw/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if !item.PassIndexed || !item.PassEmbedded || !item.PassParsed {
		t.Fatalf("raw passes were reset: %+v", item)
	}
	if item.PassSummarized || item.PassExtracted || item.PassWritten || item.SummaryPath != "" {
		t.Fatalf("LLM passes were not reset: %+v", item)
	}
}

func TestDiffForceMarksUnchangedSourcesModified(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "raw", "a.md")
	if err := os.WriteFile(path, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	mf := manifest.New()
	mf.AddSource("raw/a.md", hash, "article", 9)
	cfg := &config.Config{Sources: []config.Source{{Path: "raw", Type: "article"}}}

	regular, err := Diff(dir, cfg, mf)
	if err != nil {
		t.Fatalf("regular Diff: %v", err)
	}
	if len(regular.Modified) != 0 {
		t.Fatalf("regular diff unexpectedly modified %d sources", len(regular.Modified))
	}

	forced, err := Diff(dir, cfg, mf, true)
	if err != nil {
		t.Fatalf("forced Diff: %v", err)
	}
	if len(forced.Modified) != 1 || forced.Modified[0].Path != "raw/a.md" {
		t.Fatalf("forced diff should modify unchanged source: %+v", forced.Modified)
	}
}

func TestDiffNeverTreatsPurposeAsSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, PurposeFilename), []byte("Project purpose"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: []config.Source{{Path: ".", Type: "article"}}}
	diff, err := Diff(dir, cfg, manifest.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range diff.Added {
		if filepath.Base(source.Path) == PurposeFilename {
			t.Fatalf("purpose.md was ingested as a source: %+v", source)
		}
	}
}

func TestDiffNeverTreatsIndexIntroAsSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IndexIntroFilename), []byte("## About\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: []config.Source{{Path: ".", Type: "article"}}}
	diff, err := Diff(dir, cfg, manifest.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range diff.Added {
		if filepath.Base(source.Path) == IndexIntroFilename {
			t.Fatalf("index_intro.md was ingested as a source: %+v", source)
		}
	}
}
