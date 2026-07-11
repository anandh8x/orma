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

func TestAliases(t *testing.T) {
	r := Apply("ssh user@192.168.1.10", Options{
		Aliases: map[string]string{"192.168.1.10": "10.0.0.5"},
	})
	if !r.Changed || r.Adapted != "ssh user@10.0.0.5" {
		t.Fatalf("%+v", r)
	}
}
