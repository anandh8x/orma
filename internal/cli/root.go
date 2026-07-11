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

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local setup (placeholder until store lands)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("orma doctor: ok (scaffold only, no store yet)")
			return nil
		},
	}
}
