package noise

import (
	"path/filepath"
	"strings"
)

// IsNoiseCommand reports whether cmd matches a configured noise entry.
// Matching is case-sensitive exact match after trim, or first token for single-token entries.
func IsNoiseCommand(cmd string, noiseList []string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	for _, n := range noiseList {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if cmd == n {
			return true
		}
		// single-token noise: match first argv only (e.g. "ls" matches "ls -la")
		if !strings.Contains(n, " ") {
			first, _, _ := strings.Cut(cmd, " ")
			base := filepath.Base(first)
			if first == n || base == n {
				return true
			}
		}
	}
	return false
}

// CWDIgnored reports whether cwd matches any glob in ignore list.
func CWDIgnored(cwd string, globs []string) bool {
	if cwd == "" || len(globs) == 0 {
		return false
	}
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		ok, err := filepath.Match(g, cwd)
		if err == nil && ok {
			return true
		}
		// also try match on base and as suffix pattern **/x
		if strings.Contains(cwd, strings.Trim(g, "*")) && strings.Contains(g, "*") {
			// simple contains fallback for patterns like **/secrets/**
			trimmed := strings.Trim(g, "*/")
			if trimmed != "" && strings.Contains(cwd, trimmed) {
				return true
			}
		}
	}
	return false
}
