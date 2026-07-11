package embed

import (
	"context"
	"database/sql"
	"fmt"
)

// ProcessQueue embeds pending queue items. Returns how many were processed.
func ProcessQueue(ctx context.Context, db *sql.DB, emb Embedder, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, ref_type, ref_id FROM embed_queue
		ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type item struct{ id, refType, refID string }
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.refType, &it.refID); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	n := 0
	for _, it := range items {
		text := loadText(ctx, db, it.refType, it.refID)
		if text == "" {
			_, _ = db.ExecContext(ctx, `DELETE FROM embed_queue WHERE id = ?`, it.id)
			continue
		}
		vec, err := emb.Embed(ctx, text)
		if err != nil {
			return n, err
		}
		if err := SaveEmbedding(ctx, db, it.refType, it.refID, emb.Name(), vec); err != nil {
			return n, err
		}
		_, _ = db.ExecContext(ctx, `DELETE FROM embed_queue WHERE id = ?`, it.id)
		n++
	}
	return n, nil
}

func loadText(ctx context.Context, db *sql.DB, refType, refID string) string {
	switch refType {
	case "workflow":
		var body string
		_ = db.QueryRowContext(ctx, `SELECT COALESCE(body,'') FROM workflows WHERE id = ?`, refID).Scan(&body)
		return body
	case "note":
		var text string
		_ = db.QueryRowContext(ctx, `SELECT text FROM notes WHERE id = ?`, refID).Scan(&text)
		return text
	default:
		return ""
	}
}

// QueueStats returns pending embed jobs.
func QueueStats(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM embed_queue`).Scan(&n)
	return n, err
}

// EnsureReady is a small helper for callers.
func EnsureReady(modelsDir string) error {
	ok, err := EnsureModel(modelsDir)
	if err != nil {
		return err
	}
	if !ok || !ModelReady(modelsDir) {
		return fmt.Errorf("embed model not ready in %s", modelsDir)
	}
	return nil
}
