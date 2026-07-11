package adapt

import (
	"path/filepath"
	"sort"
	"strings"
)

// Result is one adapted step.
type Result struct {
	Original string
	Adapted  string
	Changed  bool
}

// Options controls rewriting.
type Options struct {
	OldProject string
	NewProject string
	CWD        string
	// Aliases maps old host/path token -> new token (longest keys first).
	Aliases map[string]string
}

// Paths rewrites project paths and optional aliases.
func Paths(command, oldProject, newProject, cwd string) Result {
	return Apply(command, Options{OldProject: oldProject, NewProject: newProject, CWD: cwd})
}

// Apply rewrites command using Options.
func Apply(command string, opt Options) Result {
	out := command
	changed := false

	if opt.OldProject != "" && opt.NewProject != "" && opt.OldProject != opt.NewProject {
		if strings.Contains(out, opt.OldProject) {
			out = strings.ReplaceAll(out, opt.OldProject, opt.NewProject)
			changed = true
		}
	}

	if len(opt.Aliases) > 0 {
		// longest keys first to avoid partial replacements
		keys := make([]string, 0, len(opt.Aliases))
		for k := range opt.Aliases {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
		for _, k := range keys {
			if k == "" {
				continue
			}
			v := opt.Aliases[k]
			if strings.Contains(out, k) && k != v {
				out = strings.ReplaceAll(out, k, v)
				changed = true
			}
		}
	}

	_ = opt.CWD
	if strings.Contains(out, "//") && !strings.Contains(out, "://") {
		// only clean path-like segments carefully; skip full command clean
		_ = filepath.Separator
	}

	return Result{Original: command, Adapted: out, Changed: changed || out != command}
}

// Workflow adapts all steps.
func Workflow(steps []string, oldProject, newProject, cwd string) []Result {
	out := make([]Result, 0, len(steps))
	for _, s := range steps {
		out = append(out, Paths(s, oldProject, newProject, cwd))
	}
	return out
}

// IsDestructive flags dangerous commands for warnings.
func IsDestructive(cmd string) bool {
	c := strings.ToLower(strings.TrimSpace(cmd))
	patterns := []string{
		"rm -rf", "rm -fr", "mkfs", "dd if=", "dd of=",
		"kubectl delete", "drop database", "drop table",
		"git push --force", "git reset --hard",
		">/dev/sd", "wipefs", "chmod -r 777 /",
		":(){", "fork bomb",
	}
	for _, p := range patterns {
		if strings.Contains(c, p) {
			return true
		}
	}
	return false
}
