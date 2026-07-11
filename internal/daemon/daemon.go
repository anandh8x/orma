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
			_, _ = embed.ProcessQueue(ctx, st.DB(), emb, 50)
			_, _ = dis.AutoDistillStale(ctx, 3*time.Minute)
			for _, dir := range watchers {
				_ = scanJSONLDir(ctx, st, cfg, dir, seen)
			}
		}
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
