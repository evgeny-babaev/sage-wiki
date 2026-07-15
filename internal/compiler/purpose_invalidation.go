package compiler

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xoai/sage-wiki/internal/config"
	"github.com/xoai/sage-wiki/internal/manifest"
	"github.com/xoai/sage-wiki/internal/memory"
	"github.com/xoai/sage-wiki/internal/ontology"
	"github.com/xoai/sage-wiki/internal/storage"
	"github.com/xoai/sage-wiki/internal/vectors"
)

// resetPurposeDerivedState removes artifacts whose selection and wording were
// produced under the previous purpose. Raw source indexes and user outputs are
// preserved; the full pipeline rebuilds summaries and concepts.
func resetPurposeDerivedState(
	projectDir string,
	cfg *config.Config,
	mf *manifest.Manifest,
	db *storage.DB,
	memStore *memory.Store,
	vecStore *vectors.Store,
	chunkStore *memory.ChunkStore,
	ontStore *ontology.Store,
) error {
	for name := range mf.Concepts {
		docID := "concept:" + name
		if err := memStore.Delete(docID); err != nil {
			return fmt.Errorf("delete concept memory %s: %w", name, err)
		}
		if err := vecStore.Delete(docID); err != nil {
			return fmt.Errorf("delete concept vector %s: %w", name, err)
		}
		if err := ontStore.DeleteEntity(name); err != nil {
			return fmt.Errorf("delete concept entity %s: %w", name, err)
		}
		if err := db.WriteTx(func(tx *sql.Tx) error {
			return chunkStore.DeleteDocChunks(tx, docID)
		}); err != nil {
			return fmt.Errorf("delete concept chunks %s: %w", name, err)
		}
	}
	for path, source := range mf.Sources {
		if source.SummaryPath == "" {
			continue
		}
		if err := memStore.Delete(path); err != nil {
			return fmt.Errorf("delete source summary memory %s: %w", path, err)
		}
		if err := vecStore.Delete(path); err != nil {
			return fmt.Errorf("delete source summary vector %s: %w", path, err)
		}
	}

	for _, subdir := range []string{"summaries", "concepts"} {
		dir := filepath.Join(projectDir, cfg.Output, subdir)
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("recreate %s: %w", dir, err)
		}
	}

	for path, source := range mf.Sources {
		source.CompiledAt = ""
		source.SummaryPath = ""
		source.ConceptsProduced = nil
		source.ChunkCount = 0
		source.Status = "pending"
		mf.Sources[path] = source
	}
	mf.Concepts = make(map[string]manifest.Concept)
	return nil
}
