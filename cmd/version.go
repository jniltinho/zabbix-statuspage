package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	BuildDate = "Unknown"
	GitCommit = "Unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Zabbix-Statuspage\n")
			fmt.Fprintf(cmd.OutOrStdout(), " Version:    %s\n", Version)
			fmt.Fprintf(cmd.OutOrStdout(), " Build Time: %s\n", BuildDate)
			fmt.Fprintf(cmd.OutOrStdout(), " Git Commit: %s\n", GitCommit)
			return nil
		},
	}
}
