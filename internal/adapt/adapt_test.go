package adapt

import "testing"

func TestPathsRewriteProject(t *testing.T) {
	r := Paths(
		"cat /home/a/proj/foo.txt",
		"/home/a/proj",
		"/home/b/proj",
		"/home/b/proj",
	)
	if !r.Changed {
		t.Fatal("expected change")
	}
	if r.Adapted != "cat /home/b/proj/foo.txt" {
		t.Fatalf("got %q", r.Adapted)
	}
}

func TestDestructive(t *testing.T) {
	if !IsDestructive("rm -rf /tmp/x") {
		t.Fatal("expected destructive")
	}
	if IsDestructive("ls") {
		t.Fatal("ls not destructive")
	}
}
