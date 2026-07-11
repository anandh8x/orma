package history

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
)

// ImportShellHist imports bash/zsh history files.
func ImportShellHist(ctx context.Context, st *store.Store, cfg *config.Config, limit int) (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	files := []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".bash_history"),
	}
	svc := &ingest.Service{Store: st, Config: cfg}
	n := 0
	for _, f := range files {
		c, err := importFile(ctx, svc, f, limit-n)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return n, err
		}
		n += c
		if limit > 0 && n >= limit {
			break
		}
	}
	return n, nil
}

func importFile(ctx context.Context, svc *ingest.Service, path string, limit int) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	n := 0
	// zsh extended: : timestamp:0;command
	for sc.Scan() {
		line := sc.Text()
		cmd, ts := parseHistLine(line)
		if cmd == "" {
			continue
		}
		ev := map[string]any{
			"schema":  "orma.event.v1",
			"ts":      ts.UTC().Format(time.RFC3339Nano),
			"actor":   "human",
			"kind":    "shell",
			"source":  "import-hist",
			"command": cmd,
			"outcome": "unknown",
		}
		b, _ := json.Marshal(ev)
		if _, err := svc.IngestOne(ctx, b); err != nil {
			continue
		}
		n++
		if limit > 0 && n >= limit {
			break
		}
	}
	return n, sc.Err()
}

func parseHistLine(line string) (string, time.Time) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", time.Time{}
	}
	// zsh: : 1234567890:0;cmd
	if strings.HasPrefix(line, ": ") {
		rest := line[2:]
		semi := strings.Index(rest, ";")
		if semi > 0 {
			meta := rest[:semi]
			cmd := rest[semi+1:]
			parts := strings.SplitN(meta, ":", 2)
			if len(parts) >= 1 {
				var sec int64
				if _, err := fmt.Sscanf(parts[0], "%d", &sec); err == nil && sec > 0 {
					return cmd, time.Unix(sec, 0)
				}
			}
			return cmd, time.Now().UTC()
		}
	}
	return line, time.Now().UTC()
}

// CountHistLines estimates lines available to import.
func CountHistLines() (int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, name := range []string{".zsh_history", ".bash_history"} {
		path := filepath.Join(home, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.TrimSpace(sc.Text()) != "" {
				total++
			}
		}
		_ = f.Close()
	}
	return total, nil
}
