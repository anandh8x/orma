package shellembed

import (
	"strings"
	"testing"
)

func TestScriptInjectsBinary(t *testing.T) {
	s, err := Script("zsh", "/home/u/.local/bin/orma")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "/home/u/.local/bin/orma") {
		t.Fatal("missing binary path")
	}
	if strings.Contains(s, "__ORMA_BIN__") {
		t.Fatal("placeholder left")
	}
	s2, err := Script("bash", "/opt/orma")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s2, "/opt/orma") {
		t.Fatal("bash missing path")
	}
}

func TestFishScript(t *testing.T) {
	s, err := Script("fish", "/bin/orma")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "/bin/orma") {
		t.Fatal(s)
	}
}

func TestBadShell(t *testing.T) {
	if _, err := Script("xonsh", "orma"); err == nil {
		t.Fatal("expected error")
	}
}
