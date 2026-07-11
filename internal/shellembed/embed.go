package shellembed

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed zsh.zsh bash.bash fish.fish
var files embed.FS

// Script returns the integration script for shell ("zsh", "bash", or "fish"),
// with ormaBin substituted so hooks call this install even if PATH differs.
func Script(shell, ormaBin string) (string, error) {
	var name string
	switch shell {
	case "zsh":
		name = "zsh.zsh"
	case "bash":
		name = "bash.bash"
	case "fish":
		name = "fish.fish"
	default:
		return "", fmt.Errorf("unsupported shell %q (zsh|bash|fish)", shell)
	}
	b, err := files.ReadFile(name)
	if err != nil {
		return "", err
	}
	if ormaBin == "" {
		ormaBin = "orma"
	}
	// Quote for shell single-quoted string safety
	quoted := strings.ReplaceAll(ormaBin, "'", `'\''`)
	out := string(b)
	out = strings.ReplaceAll(out, "__ORMA_BIN__", quoted)
	return out, nil
}
