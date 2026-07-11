package clipboard

import (
	"fmt"
	"os"
	"os/exec"
)

// Copy tries pbcopy, wl-copy, xclip in order.
func Copy(text string) error {
	candidates := [][]string{
		{"pbcopy"},
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			continue
		}
		if err := cmd.Start(); err != nil {
			continue
		}
		_, _ = stdin.Write([]byte(text))
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("no clipboard tool (pbcopy/wl-copy/xclip)")
}

// CopyOrPrint copies when possible, always prints text.
func CopyOrPrint(text string) {
	fmt.Println(text)
	if err := Copy(text); err != nil {
		fmt.Fprintln(os.Stderr, "clipboard:", err.Error())
	}
}
