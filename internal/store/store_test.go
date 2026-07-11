package store

import (
	"path/filepath"
	"testing"
)

func TestOpenMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orma.db")
	s, err := Open(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ver, err := s.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver != schemaVersion {
		t.Fatalf("ver=%d", ver)
	}

	// reopen
	s2, err := Open(path, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	ver2, err := s2.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if ver2 != schemaVersion {
		t.Fatalf("ver2=%d", ver2)
	}
}
