package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ishell",
	Short: "Interactive shell session manager for AI agents",
	Long:  `ishell enables AI agents to interact with persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.)`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(readCmd)
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(execCmd)
	rootCmd.AddCommand(killCmd)
}
