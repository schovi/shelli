package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var stopJsonFlag bool

func init() {
	stopCmd.Flags().BoolVar(&stopJsonFlag, "json", false, "Output as JSON")
}

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a session (keeps output accessible)",
	Long:  `Stop a running session. The process is terminated but output remains accessible for reading.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if err := client.Stop(name); err != nil {
		return err
	}

	if stopJsonFlag {
		out := map[string]interface{}{
			"name":   name,
			"status": "stopped",
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Stopped session %q\n", name)
	}
	return nil
}
