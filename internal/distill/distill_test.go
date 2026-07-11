package distill_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/distill"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
	"github.com/anandh8x/orma/internal/workflow"
)

func TestDistillSessionCreatesWorkflowAndFix(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	st, err := store.Open(filepath.Join(dir, "orma.db"), 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := &ingest.Service{Store: st, Config: cfg}
	ctx := context.Background()
	events := []string{
		`{"schema":"orma.event.v1","ts":"2026-07-11T10:00:00Z","actor":"human","kind":"shell","source":"shell","command":"false","exit_code":1,"cwd":"` + dir + `"}`,
		`{"schema":"orma.event.v1","ts":"2026-07-11T10:00:01Z","actor":"human","kind":"shell","source":"shell","command":"true","exit_code":0,"cwd":"` + dir + `"}`,
	}
	var sid string
	for _, raw := range events {
		res, err := svc.IngestOne(ctx, []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		sid = res.SessionID
	}

	d := &distill.Service{Store: st, Workflow: &workflow.Service{Store: st}}
	w, err := d.DistillSession(ctx, sid, "probe")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Steps) < 2 {
		t.Fatalf("steps=%d", len(w.Steps))
	}

	// wait a tick for same session
	time.Sleep(time.Millisecond)

	var fixes int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM fixes`).Scan(&fixes); err != nil {
		t.Fatal(err)
	}
	if fixes < 1 {
		t.Fatalf("expected fix rows, got %d", fixes)
	}
}
