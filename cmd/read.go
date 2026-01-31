package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read <name>",
	Short: "Read output from a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runRead,
}

var readModeFlag string
var readJsonFlag bool

func init() {
	readCmd.Flags().StringVar(&readModeFlag, "mode", "new", "Read mode: all|new")
	readCmd.Flags().BoolVar(&readJsonFlag, "json", false, "Output as JSON")
}

func runRead(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	output, pos, err := client.Read(name, readModeFlag)
	if err != nil {
		return err
	}

	if readJsonFlag {
		out := map[string]interface{}{
			"output":   output,
			"position": pos,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(output)
	}

	return nil
}
