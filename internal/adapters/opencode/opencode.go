package opencode

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

// SessionDirs returns likely OpenCode data dirs.
func SessionDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".opencode"),
		filepath.Join(home, ".local", "share", "opencode"),
	}
}

// Backfill imports json/jsonl session-like files.
func Backfill(ctx context.Context, st *store.Store, cfg *config.Config) (int, error) {
	svc := &ingest.Service{Store: st, Config: cfg}
	n := 0
	found := false
	for _, dir := range SessionDirs() {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		found = true
		_ = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi == nil || fi.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".jsonl" && ext != ".json" {
				return nil
			}
			if err := daemon.ImportAgentFile(ctx, svc, path); err == nil {
				n++
			}
			return nil
		})
	}
	if !found {
		return 0, fmt.Errorf("opencode data dirs not found")
	}
	return n, nil
}
