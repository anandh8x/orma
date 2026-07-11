package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version stays "dev" in the source tree on purpose.
// Do not bump it on normal commits. Releases set it once via:
//
//	go build -ldflags "-X github.com/anandh8x/orma/internal/cli.version=v1.2.3" ./cmd/orma
//
// Prefer git tags for product versions, not every PR.
var version = "dev"

// Execute runs the root command.
func Execute() error {
	root := &cobra.Command{
		Use:           "orma",
		Short:         "Local operational memory for your shell and coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	registerAll(root)
	return root.Execute()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print orma version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version)
			return nil
		},
	}
}
