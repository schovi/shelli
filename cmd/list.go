package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE:  runList,
}

var listJsonFlag bool

func init() {
	listCmd.Flags().BoolVar(&listJsonFlag, "json", false, "Output as JSON")
}

func runList(cmd *cobra.Command, args []string) error {
	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	sessions, err := client.List()
	if err != nil {
		return err
	}

	if listJsonFlag {
		data, err := json.MarshalIndent(sessions, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(data))
	} else {
		if len(sessions) == 0 {
			fmt.Println("No sessions")
			return nil
		}
		for _, s := range sessions {
			fmt.Printf("%s\t%s\t%d\t%s\n", s.Name, s.State, s.PID, s.Command)
		}
	}

	return nil
}
