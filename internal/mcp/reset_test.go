package mcp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/xoai/sage-wiki/internal/manifest"
	"github.com/xoai/sage-wiki/internal/wiki"
)

func TestResetKnowledgeRequiresConfirmation(t *testing.T) {
	dir := t.TempDir()
	if err := wiki.InitGreenfield(dir, "test", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw", "keep.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	result := srv.CallTool(context.Background(), "wiki_reset_knowledge", mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: "wiki_reset_knowledge"},
	})
	if !result.IsError {
		t.Fatal("reset without confirmation should fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "raw", "keep.md")); err != nil {
		t.Fatalf("source changed without confirmation: %v", err)
	}
}

func TestResetKnowledgeClearsDerivedStateAndPreservesProjectInstructions(t *testing.T) {
	dir := t.TempDir()
	if err := wiki.InitGreenfield(dir, "test", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	purpose := "# Purpose\n\nKeep the project goal.\n"
	intro := "# Shared Wiki\n\nRead this first.\n"
	if err := os.WriteFile(filepath.Join(dir, "purpose.md"), []byte(purpose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index_intro.md"), []byte(intro), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "raw", "source.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "wiki", "summaries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "wiki", "concepts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiki", "summaries", "source.md"), []byte("summary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiki", "concepts", "concept.md"), []byte("concept"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wiki", "CHANGELOG.md"), []byte("changes"), 0o644); err != nil {
		t.Fatal(err)
	}

	mf := manifest.New()
	mf.AddSource("raw/source.md", "hash", "article", 6)
	mf.AddConcept("concept", "wiki/concepts/concept.md", []string{"raw/source.md"})
	if err := mf.Save(filepath.Join(dir, ".manifest.json")); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	seedKnowledgeTables(t, srv)
	result := srv.CallTool(context.Background(), "wiki_reset_knowledge", mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Name:      "wiki_reset_knowledge",
			Arguments: map[string]any{"confirm": true},
		},
	})
	if result.IsError {
		t.Fatalf("reset failed: %v", result.Content)
	}

	for _, path := range []string{
		filepath.Join(dir, "raw", "source.md"),
		filepath.Join(dir, "wiki", "summaries", "source.md"),
		filepath.Join(dir, "wiki", "concepts", "concept.md"),
		filepath.Join(dir, "wiki", "CHANGELOG.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected removed path %s, got %v", path, err)
		}
	}
	for path, expected := range map[string]string{
		filepath.Join(dir, "purpose.md"):     purpose,
		filepath.Join(dir, "index_intro.md"): intro,
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Errorf("preserved file %s changed: %q, %v", path, data, err)
		}
	}

	resetManifest, err := manifest.Load(filepath.Join(dir, ".manifest.json"))
	if err != nil || resetManifest.SourceCount() != 0 || resetManifest.ConceptCount() != 0 {
		t.Fatalf("manifest not reset: %+v, %v", resetManifest, err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "wiki", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "# Shared Wiki") || !strings.Contains(string(index), "_No compiled concepts._") || !strings.Contains(string(index), "_No compiled summaries._") {
		t.Fatalf("unexpected reset index: %s", index)
	}

	for _, table := range []string{
		"confirmation_sources", "pending_outputs", "pending_questions_vec",
		"relations", "entities", "chunks_fts", "chunks_meta", "vec_chunks",
		"entries", "vec_entries", "compile_items", "learnings",
	} {
		var count int
		if err := srv.db.ReadDB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("table %s still has %d rows", table, count)
		}
	}
}

func seedKnowledgeTables(t *testing.T, srv *Server) {
	t.Helper()
	statements := []string{
		`INSERT INTO entries(id, content, tags, article_path) VALUES ('entry', 'content', '', 'wiki/concepts/concept.md')`,
		`INSERT INTO vec_entries(id, embedding, dimensions) VALUES ('entry', X'00', 1)`,
		`INSERT INTO entities(id, type, name) VALUES ('source', 'source', 'Source')`,
		`INSERT INTO entities(id, type, name) VALUES ('concept', 'concept', 'Concept')`,
		`INSERT INTO relations(id, source_id, target_id, relation) VALUES ('rel', 'source', 'concept', 'supports')`,
		`INSERT INTO chunks_meta(chunk_id, doc_id, chunk_index, content) VALUES ('chunk', 'entry', 0, 'content')`,
		`INSERT INTO chunks_fts(chunk_id, heading, content) VALUES ('chunk', '', 'content')`,
		`INSERT INTO vec_chunks(chunk_id, doc_id, embedding, dimensions) VALUES ('chunk', 'entry', X'00', 1)`,
		`INSERT INTO learnings(id, type, content) VALUES ('learning', 'gotcha', 'content')`,
		`INSERT INTO compile_items(source_path) VALUES ('raw/source.md')`,
		`INSERT INTO pending_outputs(id, question, question_hash, answer, answer_hash, file_path, created_at) VALUES ('output', 'q', 'qh', 'a', 'ah', 'wiki/pending.md', 'now')`,
		`INSERT INTO confirmation_sources(output_id, chunk_ids, answer_hash, confirmed_at) VALUES ('output', 'chunk', 'ah', 'now')`,
		`INSERT INTO pending_questions_vec(question_hash, embedding, dimensions) VALUES ('qh', X'00', 1)`,
	}
	if err := srv.db.WriteTx(func(tx *sql.Tx) error {
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}
