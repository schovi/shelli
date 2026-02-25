package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/schovi/shelli/internal/vterm"
	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <name> <input>",
	Short: "Send command and wait for result",
	Long: `Send a command to a session and wait for the result.

Sends the input as literal text with a newline appended, then waits for output
to settle. Escape sequences like \n are NOT interpreted - they're passed to
the shell as-is (the shell may interpret them, e.g., echo -e).

For precise control over escape sequences, use 'send' instead.

By default waits for 500ms of silence. Use --wait for pattern matching.`,
	Args: cobra.MinimumNArgs(2),
	RunE: runExec,
}

var (
	execWaitFlag      string
	execSettleFlag    int
	execTimeoutFlag   int
	execStripAnsiFlag bool
	execJsonFlag      bool
)

func init() {
	execCmd.Flags().StringVar(&execWaitFlag, "wait", "", "Wait for regex pattern match")
	execCmd.Flags().IntVar(&execSettleFlag, "settle", 500, "Wait for N ms of silence (default 500)")
	execCmd.Flags().IntVar(&execTimeoutFlag, "timeout", 10, "Max wait time in seconds")
	execCmd.Flags().BoolVar(&execStripAnsiFlag, "strip-ansi", false, "Strip ANSI escape codes")
	execCmd.Flags().BoolVar(&execJsonFlag, "json", false, "Output as JSON")
}

func runExec(cmd *cobra.Command, args []string) error {
	name := args[0]
	input := strings.Join(args[1:], " ")

	hasWait := execWaitFlag != ""
	hasSettle := cmd.Flags().Changed("settle")

	if hasWait && hasSettle {
		return fmt.Errorf("--wait and --settle are mutually exclusive")
	}

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	var pattern string
	var settleMs int

	if hasWait {
		pattern = execWaitFlag
	} else {
		settleMs = execSettleFlag
	}

	result, err := client.Exec(name, daemon.ExecOptions{
		Input:       input,
		SettleMs:    settleMs,
		WaitPattern: pattern,
		TimeoutSec:  execTimeoutFlag,
	})
	if err != nil {
		if result == nil || result.Output == "" {
			return err
		}
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	output := result.Output
	if execStripAnsiFlag {
		output = vterm.StripDefault(output)
	}

	if execJsonFlag {
		out := map[string]interface{}{
			"input":    result.Input,
			"output":   output,
			"position": result.Position,
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
