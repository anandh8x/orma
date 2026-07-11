package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/daemon"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
)

// DefaultSessionsDir returns ~/.codex/sessions.
func DefaultSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

// Backfill imports existing session JSONL files once.
func Backfill(ctx context.Context, st *store.Store, cfg *config.Config) (int, error) {
	dir := DefaultSessionsDir()
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return 0, fmt.Errorf("codex sessions not found at %s", dir)
	}
	svc := &ingest.Service{Store: st, Config: cfg}
	n := 0
	err = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if err := daemon.ImportAgentFile(ctx, svc, path); err == nil {
			n++
		}
		return nil
	})
	return n, err
}
