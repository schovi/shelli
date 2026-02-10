package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new interactive session",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreate,
}

var (
	createCmdFlag  string
	createJsonFlag bool
	createEnvFlag  []string
	createCwdFlag  string
	createColsFlag int
	createRowsFlag int
	createTUIFlag  bool
)

func init() {
	createCmd.Flags().StringVar(&createCmdFlag, "cmd", "", "Command to run (default: $SHELL)")
	createCmd.Flags().BoolVar(&createJsonFlag, "json", false, "Output as JSON")
	createCmd.Flags().StringArrayVar(&createEnvFlag, "env", nil, "Set environment variable (KEY=VALUE), can be repeated")
	createCmd.Flags().StringVar(&createCwdFlag, "cwd", "", "Set working directory")
	createCmd.Flags().IntVar(&createColsFlag, "cols", 80, "Terminal columns")
	createCmd.Flags().IntVar(&createRowsFlag, "rows", 24, "Terminal rows")
	createCmd.Flags().BoolVar(&createTUIFlag, "tui", false, "Enable TUI mode (truncate buffer on screen clear)")
}

func runCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	data, err := client.Create(name, daemon.CreateOptions{
		Command: createCmdFlag,
		Env:     createEnvFlag,
		Cwd:     createCwdFlag,
		Cols:    createColsFlag,
		Rows:    createRowsFlag,
		TUIMode: createTUIFlag,
	})
	if err != nil {
		return err
	}

	if createJsonFlag {
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(out))
	} else {
		fmt.Printf("Created session %q (pid: %.0f, cmd: %s)\n",
			data["name"], data["pid"], data["command"])
	}

	return nil
}
