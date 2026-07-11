package adapt

import (
	"path/filepath"
	"strings"
)

// Result is one adapted step.
type Result struct {
	Original string
	Adapted  string
	Changed  bool
}

// Paths rewrites absolute paths that sit under oldProject to newProject / cwd.
func Paths(command, oldProject, newProject, cwd string) Result {
	out := command
	changed := false

	if oldProject != "" && newProject != "" && oldProject != newProject {
		if strings.Contains(out, oldProject) {
			out = strings.ReplaceAll(out, oldProject, newProject)
			changed = true
		}
	}

	// Relative paths starting with ./ stay; if cwd differs and we have old project file refs, done above.

	// Rewrite home-style if new home differs: skip unless obvious
	_ = cwd

	// Normalize double slashes lightly
	if strings.Contains(out, "//") && !strings.Contains(out, "://") {
		out2 := filepath.Clean(out)
		_ = out2
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
	}
	for _, p := range patterns {
		if strings.Contains(c, p) {
			return true
		}
	}
	return false
}
