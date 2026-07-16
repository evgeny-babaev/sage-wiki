package storage

import "database/sql"

// ResetKnowledge removes all project knowledge while preserving the database
// schema. Keeping the existing connections alive is important for MCP servers.
func (db *DB) ResetKnowledge() error {
	return db.WriteTx(func(tx *sql.Tx) error {
		for _, table := range []string{
			"confirmation_sources",
			"pending_outputs",
			"pending_questions_vec",
			"relations",
			"entities",
			"chunks_fts",
			"chunks_meta",
			"vec_chunks",
			"entries",
			"vec_entries",
			"compile_items",
			"learnings",
		} {
			if _, err := tx.Exec("DELETE FROM " + table); err != nil {
				return err
			}
		}
		return nil
	})
}
