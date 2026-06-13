package cmd

import (
	"io/fs"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "zabbix-statuspage",
	Short:         "Public status page powered by Zabbix",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute(webFS fs.FS) error {
	rootCmd.AddCommand(newServeCmd(webFS))
	rootCmd.AddCommand(newVersionCmd())
	return rootCmd.Execute()
}
