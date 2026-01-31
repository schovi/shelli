package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/schovi/ishell/internal/ansi"
	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <name> <input>",
	Short: "Send input and wait for result",
	Long: `Send input to a session and wait for the result.

Sends the input with a newline, then waits for output to settle.
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

	// Get current position before sending
	_, startPos, err := client.Read(name, "all")
	if err != nil {
		return err
	}

	// Send input with newline
	if err := client.Send(name, input, true); err != nil {
		return err
	}

	// Determine wait mode
	var pattern string
	var settleMs int

	if hasWait {
		pattern = execWaitFlag
		settleMs = 0
	} else {
		pattern = ""
		settleMs = execSettleFlag
	}

	// Wait for result
	output, pos, err := execBlockingRead(client, name, startPos, pattern, settleMs, execTimeoutFlag)
	if err != nil {
		return err
	}

	if execStripAnsiFlag {
		output = ansi.Strip(output)
	}

	if execJsonFlag {
		out := map[string]interface{}{
			"input":    input,
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

func execBlockingRead(client *daemon.Client, name string, startPos int, pattern string, settleMs, timeoutSec int) (string, int, error) {
	var re *regexp.Regexp
	if pattern != "" {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return "", 0, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	timeout := time.Duration(timeoutSec) * time.Second
	deadline := time.Now().Add(timeout)
	pollInterval := 50 * time.Millisecond
	settleDuration := time.Duration(settleMs) * time.Millisecond

	var lastPos int = startPos
	var lastChangeTime = time.Now()

	for time.Now().Before(deadline) {
		output, pos, err := client.Read(name, "all")
		if err != nil {
			return "", 0, err
		}

		// Check for new output
		if pos != lastPos {
			lastPos = pos
			lastChangeTime = time.Now()
		}

		// Get output since send
		newOutput := ""
		if pos > startPos {
			newOutput = output[startPos:]
		}

		// Pattern mode: check if pattern matches in new output
		if re != nil && re.MatchString(newOutput) {
			return newOutput, pos, nil
		}

		// Settle mode: check if output has settled
		if settleMs > 0 && pos > startPos && time.Since(lastChangeTime) >= settleDuration {
			return newOutput, pos, nil
		}

		time.Sleep(pollInterval)
	}

	// Timeout - get final output
	output, pos, _ := client.Read(name, "all")
	newOutput := ""
	if pos > startPos {
		newOutput = output[startPos:]
	}

	if re != nil {
		return newOutput, pos, fmt.Errorf("timeout waiting for pattern %q", pattern)
	}
	return newOutput, pos, fmt.Errorf("timeout waiting for output to settle")
}
