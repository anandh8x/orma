package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
	_ "modernc.org/sqlite"
)

// DefaultAtuinDB is the usual Atuin history database path.
func DefaultAtuinDB() string {
	home, _ := os.UserHomeDir()
	// XDG data
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "atuin", "history.db")
	}
	return filepath.Join(home, ".local", "share", "atuin", "history.db")
}

// ImportAtuin reads command history from an Atuin sqlite DB (read-only).
func ImportAtuin(ctx context.Context, st *store.Store, cfg *config.Config, dbPath string, limit int) (int, error) {
	if dbPath == "" {
		dbPath = DefaultAtuinDB()
	}
	if _, err := os.Stat(dbPath); err != nil {
		return 0, fmt.Errorf("atuin db not found: %s", dbPath)
	}

	// open read-only
	dsn := fmt.Sprintf("file:%s?mode=ro", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// Atuin schema varies; try common column names.
	q := `SELECT command, COALESCE(cwd,''), timestamp, COALESCE(exit,-1)
	      FROM history ORDER BY timestamp DESC`
	if limit > 0 {
		q = fmt.Sprintf("%s LIMIT %d", q, limit)
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		// older schema without exit
		q = `SELECT command, COALESCE(cwd,''), timestamp, -1 FROM history ORDER BY timestamp DESC`
		if limit > 0 {
			q = fmt.Sprintf("%s LIMIT %d", q, limit)
		}
		rows, err = db.QueryContext(ctx, q)
		if err != nil {
			return 0, fmt.Errorf("query atuin history: %w", err)
		}
	}
	defer rows.Close()

	svc := &ingest.Service{Store: st, Config: cfg}
	n := 0
	for rows.Next() {
		var cmd, cwd string
		var ts any
		var exit int
		if err := rows.Scan(&cmd, &cwd, &ts, &exit); err != nil {
			continue
		}
		if cmd == "" {
			continue
		}
		t := parseAtuinTS(ts)
		ev := map[string]any{
			"schema":  "orma.event.v1",
			"ts":      t.UTC().Format(time.RFC3339Nano),
			"actor":   "human",
			"kind":    "shell",
			"source":  "import-atuin",
			"command": cmd,
			"cwd":     cwd,
		}
		if exit >= 0 {
			ev["exit_code"] = exit
		} else {
			ev["outcome"] = "unknown"
		}
		b, _ := json.Marshal(ev)
		if _, err := svc.IngestOne(ctx, b); err != nil {
			continue
		}
		n++
	}
	return n, rows.Err()
}

func parseAtuinTS(v any) time.Time {
	switch t := v.(type) {
	case int64:
		// nanoseconds or seconds
		if t > 1e15 {
			return time.Unix(0, t)
		}
		if t > 1e12 {
			return time.Unix(0, t*int64(time.Millisecond))
		}
		return time.Unix(t, 0)
	case float64:
		return parseAtuinTS(int64(t))
	case []byte:
		var n int64
		fmt.Sscanf(string(t), "%d", &n)
		return parseAtuinTS(n)
	case string:
		var n int64
		fmt.Sscanf(t, "%d", &n)
		if n > 0 {
			return parseAtuinTS(n)
		}
		if tm, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return tm
		}
	}
	return time.Now().UTC()
}
