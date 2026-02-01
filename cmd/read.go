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
	readHeadFlag      int
	readTailFlag      int
	readWaitFlag      string
	readSettleFlag    int
	readTimeoutFlag   int
	readStripAnsiFlag bool
	readJsonFlag      bool
)

func init() {
	readCmd.Flags().BoolVar(&readAllFlag, "all", false, "Read all output from session start")
	readCmd.Flags().IntVar(&readHeadFlag, "head", 0, "Return first N lines of buffer")
	readCmd.Flags().IntVar(&readTailFlag, "tail", 0, "Return last N lines of buffer")
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

	modeCount := 0
	if readAllFlag {
		modeCount++
	}
	if readHeadFlag > 0 {
		modeCount++
	}
	if readTailFlag > 0 {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("--all, --head, and --tail are mutually exclusive")
	}

	if readHeadFlag < 0 || readTailFlag < 0 {
		return fmt.Errorf("--head and --tail require positive integers")
	}

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

	headLines := readHeadFlag
	tailLines := readTailFlag

	if blocking {
		_, startPos, readErr := client.Read(name, "new", 0, 0)
		if readErr != nil {
			return readErr
		}

		output, pos, err = wait.ForOutput(
			func() (string, int, error) { return client.Read(name, "all", headLines, tailLines) },
			wait.Config{
				Pattern:       readWaitFlag,
				SettleMs:      readSettleFlag,
				TimeoutSec:    readTimeoutFlag,
				StartPosition: startPos,
			},
		)
	} else {
		mode := "new"
		if readAllFlag || readHeadFlag > 0 || readTailFlag > 0 {
			mode = "all"
		}
		output, pos, err = client.Read(name, mode, headLines, tailLines)
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
