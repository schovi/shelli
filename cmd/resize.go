package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var (
	resizeColsFlag int
	resizeRowsFlag int
	resizeJsonFlag bool
)

func init() {
	resizeCmd.Flags().IntVar(&resizeColsFlag, "cols", 0, "Terminal columns")
	resizeCmd.Flags().IntVar(&resizeRowsFlag, "rows", 0, "Terminal rows")
	resizeCmd.Flags().BoolVar(&resizeJsonFlag, "json", false, "Output as JSON")
}

var resizeCmd = &cobra.Command{
	Use:   "resize <name>",
	Short: "Resize terminal dimensions",
	Long:  `Change the terminal dimensions of a running session. At least one of --cols or --rows must be specified.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runResize,
}

func runResize(cmd *cobra.Command, args []string) error {
	name := args[0]

	if resizeColsFlag <= 0 && resizeRowsFlag <= 0 {
		return fmt.Errorf("at least one of --cols or --rows is required")
	}

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if err := client.Resize(name, resizeColsFlag, resizeRowsFlag); err != nil {
		return err
	}

	if resizeJsonFlag {
		out := map[string]interface{}{
			"name":   name,
			"status": "resized",
		}
		if resizeColsFlag > 0 {
			out["cols"] = resizeColsFlag
		}
		if resizeRowsFlag > 0 {
			out["rows"] = resizeRowsFlag
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	switch {
	case resizeColsFlag > 0 && resizeRowsFlag > 0:
		fmt.Printf("Resized session %q to %dx%d\n", name, resizeColsFlag, resizeRowsFlag)
	case resizeColsFlag > 0:
		fmt.Printf("Resized session %q cols to %d\n", name, resizeColsFlag)
	default:
		fmt.Printf("Resized session %q rows to %d\n", name, resizeRowsFlag)
	}
	return nil
}
