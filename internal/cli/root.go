package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time with -ldflags when we care.
var version = "0.0.0-dev"

// Execute runs the root command.
func Execute() error {
	root := &cobra.Command{
		Use:           "orma",
		Short:         "Local operational memory for your shell and coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newInitCmd())

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
