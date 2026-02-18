package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var killJsonFlag bool

func init() {
	killCmd.Flags().BoolVar(&killJsonFlag, "json", false, "Output as JSON")
}

var killCmd = &cobra.Command{
	Use:   "kill <name>",
	Short: "Kill a session",
	Long: `Kill a session: terminates the process (if running) and permanently deletes all stored output.

To stop a session but keep output accessible for later reading, use 'stop' instead.

This is a destructive operation and cannot be undone.`,
	Args: cobra.ExactArgs(1),
	RunE: runKill,
}

func runKill(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if err := client.Kill(name); err != nil {
		return err
	}

	if killJsonFlag {
		out := map[string]interface{}{
			"name":   name,
			"status": "killed",
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Killed session %q\n", name)
	}
	return nil
}
