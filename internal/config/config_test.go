package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	cfg.path = path
	cfg.DataDir = filepath.Join(dir, "data")
	cfg.SessionIdle = Duration{15 * time.Minute}
	cfg.Redact = true
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DataDir != cfg.DataDir {
		t.Fatalf("data_dir %q", loaded.DataDir)
	}
	if loaded.SessionIdle.Duration != 15*time.Minute {
		t.Fatalf("idle %v", loaded.SessionIdle.Duration)
	}
	if !loaded.Redact {
		t.Fatal("redact")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
