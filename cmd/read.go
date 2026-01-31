package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/ansi"
	"github.com/schovi/shelli/internal/daemon"
	"github.com/schovi/shelli/internal/wait"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read <name>",
	Short: "Read output from a session",
	Long: `Read output from a session.

By default, returns new output since last read (instant).
Use --all for all output from session start (instant).
Use --wait or --settle for blocking read (returns new output).`,
	Args: cobra.ExactArgs(1),
	RunE: runRead,
}

var (
	readAllFlag       bool
	readWaitFlag      string
	readSettleFlag    int
	readTimeoutFlag   int
	readStripAnsiFlag bool
	readJsonFlag      bool
)

func init() {
	readCmd.Flags().BoolVar(&readAllFlag, "all", false, "Read all output from session start")
	readCmd.Flags().StringVar(&readWaitFlag, "wait", "", "Wait for regex pattern match")
	readCmd.Flags().IntVar(&readSettleFlag, "settle", 0, "Wait for N ms of silence")
	readCmd.Flags().IntVar(&readTimeoutFlag, "timeout", 10, "Max wait time in seconds (for blocking modes)")
	readCmd.Flags().BoolVar(&readStripAnsiFlag, "strip-ansi", false, "Strip ANSI escape codes")
	readCmd.Flags().BoolVar(&readJsonFlag, "json", false, "Output as JSON")
}

func runRead(cmd *cobra.Command, args []string) error {
	name := args[0]

	hasWait := readWaitFlag != ""
	hasSettle := readSettleFlag > 0
	blocking := hasWait || hasSettle

	if readAllFlag && blocking {
		return fmt.Errorf("--all cannot be combined with --wait or --settle")
	}
	if hasWait && hasSettle {
		return fmt.Errorf("--wait and --settle are mutually exclusive")
	}

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	var output string
	var pos int
	var err error

	if blocking {
		_, startPos, readErr := client.Read(name, "new")
		if readErr != nil {
			return readErr
		}

		output, pos, err = wait.ForOutput(
			func() (string, int, error) { return client.Read(name, "all") },
			wait.Config{
				Pattern:       readWaitFlag,
				SettleMs:      readSettleFlag,
				TimeoutSec:    readTimeoutFlag,
				StartPosition: startPos,
			},
		)
	} else {
		mode := "new"
		if readAllFlag {
			mode = "all"
		}
		output, pos, err = client.Read(name, mode)
	}

	if err != nil {
		return err
	}

	if readStripAnsiFlag {
		output = ansi.Strip(output)
	}

	if readJsonFlag {
		out := map[string]interface{}{
			"output":   output,
			"position": pos,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Print(output)
	}

	return nil
}
