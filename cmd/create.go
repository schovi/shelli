package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new interactive session",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

var createCmdFlag string
var createJsonFlag bool

func init() {
	createCmd.Flags().StringVar(&createCmdFlag, "cmd", "", "Command to run (default: $SHELL)")
	createCmd.Flags().BoolVar(&createJsonFlag, "json", false, "Output as JSON")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	data, err := client.Create(name, createCmdFlag)
	if err != nil {
		return err
	}

	if createJsonFlag {
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("Created session %q (pid: %.0f, cmd: %s)\n",
			data["name"], data["pid"], data["command"])
	}

	return nil
}
