package cli

import "testing"

func TestVersionConstantSet(t *testing.T) {
	if version == "" {
		t.Fatal("version should not be empty")
	}
}
