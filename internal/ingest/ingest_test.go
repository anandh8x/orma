package ingest_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/anandh8x/orma/internal/config"
	"github.com/anandh8x/orma/internal/ingest"
	"github.com/anandh8x/orma/internal/store"
)

func TestIngestShellEvent(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	cfg.SessionIdle = config.Duration{Duration: 20 * time.Minute}

	st, err := store.Open(filepath.Join(dir, "orma.db"), cfg.BusyTimeoutMS)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := &ingest.Service{Store: st, Config: cfg}
	raw := []byte(`{
		"schema": "orma.event.v1",
		"ts": "2026-07-11T18:22:01Z",
		"actor": "human",
		"kind": "shell",
		"source": "shell",
		"command": "docker compose up -d",
		"cwd": "` + dir + `",
		"exit_code": 0
	}`)
	res, err := svc.IngestOne(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.EventID == "" || res.SessionID == "" {
		t.Fatalf("result=%+v", res)
	}

	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("events=%d", n)
	}
}

func TestNoiseFlagged(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	st, err := store.Open(filepath.Join(dir, "orma.db"), cfg.BusyTimeoutMS)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := &ingest.Service{Store: st, Config: cfg}
	raw := []byte(`{
		"schema": "orma.event.v1",
		"ts": "2026-07-11T18:22:01Z",
		"actor": "human",
		"kind": "shell",
		"source": "shell",
		"command": "ls -la",
		"exit_code": 0
	}`)
	res, err := svc.IngestOne(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Noise {
		t.Fatal("expected noise")
	}
	var isNoise int
	if err := st.DB().QueryRow(`SELECT is_noise FROM events WHERE id = ?`, res.EventID).Scan(&isNoise); err != nil {
		t.Fatal(err)
	}
	if isNoise != 1 {
		t.Fatalf("is_noise=%d", isNoise)
	}
}

func TestSessionGapOpensNew(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DataDir = dir
	cfg.SessionIdle = config.Duration{Duration: 20 * time.Minute}
	st, err := store.Open(filepath.Join(dir, "orma.db"), cfg.BusyTimeoutMS)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := &ingest.Service{Store: st, Config: cfg}

	e1 := []byte(`{
		"schema": "orma.event.v1",
		"ts": "2026-07-11T10:00:00Z",
		"actor": "human",
		"kind": "shell",
		"source": "shell",
		"command": "echo one",
		"cwd": "` + dir + `",
		"exit_code": 0
	}`)
	e2 := []byte(`{
		"schema": "orma.event.v1",
		"ts": "2026-07-11T11:00:00Z",
		"actor": "human",
		"kind": "shell",
		"source": "shell",
		"command": "echo two",
		"cwd": "` + dir + `",
		"exit_code": 0
	}`)
	r1, err := svc.IngestOne(context.Background(), e1)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := svc.IngestOne(context.Background(), e2)
	if err != nil {
		t.Fatal(err)
	}
	if r1.SessionID == r2.SessionID {
		t.Fatal("expected new session after idle gap")
	}
}
