package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var clearJsonFlag bool

func init() {
	clearCmd.Flags().BoolVar(&clearJsonFlag, "json", false, "Output as JSON")
}

var clearCmd = &cobra.Command{
	Use:   "clear <name>",
	Short: "Clear session output buffer",
	Long:  `Clear the output buffer of a session and reset the read position. The session continues running.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runClear,
}

func runClear(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if err := client.Clear(name); err != nil {
		return err
	}

	if clearJsonFlag {
		out := map[string]interface{}{
			"name":   name,
			"status": "cleared",
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Cleared session %q\n", name)
	}
	return nil
}
