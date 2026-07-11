package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/distill"
	"github.com/anandh8x/orma/internal/embed"
	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
)

func pidPath(dataDir string) string {
	return filepath.Join(dataDir, "daemon.pid")
}

// Status returns whether a daemon pid file points at a live process.
func Status(dataDir string) (running bool, pid int, err error) {
	b, err := os.ReadFile(pidPath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false, 0, nil
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		return false, pid, nil
	}
	return true, pid, nil
}

// WritePID stores the current process id.
func WritePID(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(pidPath(dataDir), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// RemovePID clears the pid file.
func RemovePID(dataDir string) {
	_ = os.Remove(pidPath(dataDir))
}

// Run is the long-running daemon loop.
func Run(ctx context.Context, cfg *config.Config) error {
	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}
	if err := WritePID(cfg.DataDir); err != nil {
		return err
	}
	defer RemovePID(cfg.DataDir)

	st, err := store.Open(cfg.DBPath(), cfg.BusyTimeoutMS)
	if err != nil {
		return err
	}
	defer st.Close()

	wf := &workflow.Service{Store: st}
	dis := &distill.Service{Store: st, Workflow: wf}
	emb := embed.HashEmbedder{}
	_, _ = embed.EnsureModel(embed.ModelsDir(cfg.DataDir))

	watchers := defaultWatchDirs()
	seen := map[string]int64{}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = processEmbedQueue(ctx, st, emb)
			_, _ = dis.AutoDistillStale(ctx, 3*time.Minute)
			for _, dir := range watchers {
				_ = scanJSONLDir(ctx, st, cfg, dir, seen)
			}
		}
	}
}

func processEmbedQueue(ctx context.Context, st *store.Store, emb embed.HashEmbedder) error {
	rows, err := st.DB().QueryContext(ctx, `
		SELECT id, ref_type, ref_id FROM embed_queue ORDER BY created_at ASC LIMIT 50`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type item struct{ id, refType, refID string }
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.refType, &it.refID); err != nil {
			return err
		}
		items = append(items, it)
	}
	for _, it := range items {
		text := loadText(ctx, st, it.refType, it.refID)
		if text == "" {
			_, _ = st.DB().ExecContext(ctx, `DELETE FROM embed_queue WHERE id = ?`, it.id)
			continue
		}
		vec, err := emb.Embed(ctx, text)
		if err != nil {
			continue
		}
		if err := embed.SaveEmbedding(ctx, st.DB(), it.refType, it.refID, emb.Name(), vec); err != nil {
			continue
		}
		_, _ = st.DB().ExecContext(ctx, `DELETE FROM embed_queue WHERE id = ?`, it.id)
	}
	return nil
}

func loadText(ctx context.Context, st *store.Store, refType, refID string) string {
	switch refType {
	case "workflow":
		var body string
		_ = st.DB().QueryRowContext(ctx, `SELECT COALESCE(body,'') FROM workflows WHERE id = ?`, refID).Scan(&body)
		return body
	case "note":
		var text string
		_ = st.DB().QueryRowContext(ctx, `SELECT text FROM notes WHERE id = ?`, refID).Scan(&text)
		return text
	default:
		return ""
	}
}

func defaultWatchDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, ".claude", "projects"),
		filepath.Join(home, ".local", "share", "opencode"),
		filepath.Join(home, ".opencode"),
	}
}
