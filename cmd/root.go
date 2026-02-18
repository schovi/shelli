package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "shelli",
	Short: "Shell Interactive - session manager for AI agents",
	Long: `shelli (Shell Interactive) enables AI agents to interact with persistent interactive shell sessions (REPLs, SSH, database CLIs, etc.)

Quick start:
  shelli create myshell                     # Start a shell session
  shelli exec myshell "echo hello"          # Run command and get output
  shelli read myshell                       # Read new output
  shelli stop myshell                       # Stop (keeps output)
  shelli kill myshell                       # Kill (deletes everything)`,
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
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(clearCmd)
	rootCmd.AddCommand(resizeCmd)
	rootCmd.AddCommand(versionCmd)
}
