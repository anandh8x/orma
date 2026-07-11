package ingest

import "testing"

func TestRedactBearerAndAPIKey(t *testing.T) {
	in := `curl -H "Authorization: Bearer supersecret" -H "api_key=abc123" https://x`
	out := redactString(in)
	if stringsContains(out, "supersecret") || stringsContains(out, "abc123") {
		t.Fatalf("not redacted: %s", out)
	}
	if !stringsContains(out, "REDACTED") {
		t.Fatalf("expected placeholder: %s", out)
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
