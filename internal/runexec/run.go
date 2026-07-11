package runexec

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunSteps executes adapted commands in a login shell, stopping on first failure.
func RunSteps(ctx context.Context, steps []string, confirm bool) error {
	if len(steps) == 0 {
		return fmt.Errorf("no steps")
	}
	if confirm {
		fmt.Fprintf(os.Stderr, "run %d steps? type yes: ", len(steps))
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() || strings.TrimSpace(sc.Text()) != "yes" {
			return fmt.Errorf("aborted")
		}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	for i, step := range steps {
		fmt.Fprintf(os.Stderr, "--- run step %d/%d ---\n%s\n", i+1, len(steps), step)
		cmd := exec.CommandContext(ctx, shell, "-c", step)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("step %d failed: %w", i+1, err)
		}
	}
	return nil
}
