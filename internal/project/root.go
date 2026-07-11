package project

import (
	"os"
	"path/filepath"
)

// Resolve returns project key for a cwd: walk up for .git, else cwd.
func Resolve(cwd string) string {
	if cwd == "" {
		return ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	dir := abs
	for {
		gitPath := filepath.Join(dir, ".git")
		if st, err := os.Stat(gitPath); err == nil && (st.IsDir() || st.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return abs
}
