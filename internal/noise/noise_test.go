package noise

import "testing"

func TestIsNoiseCommand(t *testing.T) {
	list := []string{"ls", "pwd", "git status"}
	cases := []struct {
		cmd  string
		want bool
	}{
		{"ls", true},
		{"ls -la", true},
		{"pwd", true},
		{"git status", true},
		{"git status -sb", false}, // exact multi-token only unless listed
		{"docker ps", false},
	}
	for _, tc := range cases {
		if got := IsNoiseCommand(tc.cmd, list); got != tc.want {
			t.Fatalf("cmd=%q got=%v want=%v", tc.cmd, got, tc.want)
		}
	}
}
