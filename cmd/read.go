package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/schovi/shelli/internal/ansi"
	"github.com/schovi/shelli/internal/daemon"
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
		output, pos, err = blockingRead(client, name, readWaitFlag, readSettleFlag, readTimeoutFlag)
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
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(output)
	}

	return nil
}

func blockingRead(client *daemon.Client, name, pattern string, settleMs, timeoutSec int) (string, int, error) {
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

	var lastOutput string
	var lastPos int
	var lastChangeTime time.Time

	// Get initial position
	_, startPos, err := client.Read(name, "new")
	if err != nil {
		return "", 0, err
	}
	lastPos = startPos
	lastChangeTime = time.Now()

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

		// Pattern mode: check if pattern matches
		if re != nil && re.MatchString(output) {
			newOutput := ""
			if pos > startPos {
				newOutput = output[startPos:]
			}
			return newOutput, pos, nil
		}

		// Settle mode: check if output has settled
		if settleMs > 0 && time.Since(lastChangeTime) >= settleDuration {
			newOutput := ""
			if pos > startPos {
				newOutput = output[startPos:]
			}
			return newOutput, pos, nil
		}

		lastOutput = output
		time.Sleep(pollInterval)
	}

	// Timeout reached
	newOutput := ""
	if lastPos > startPos {
		newOutput = lastOutput[startPos:]
	}

	if re != nil {
		return newOutput, lastPos, fmt.Errorf("timeout waiting for pattern %q", pattern)
	}
	return newOutput, lastPos, fmt.Errorf("timeout waiting for settle")
}
