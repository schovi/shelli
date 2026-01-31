package cmd

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var waitCmd = &cobra.Command{
	Use:   "wait <name> <pattern>",
	Short: "Wait for output matching a pattern",
	Args:  cobra.ExactArgs(2),
	RunE:  runWait,
}

var waitTimeoutFlag int
var waitJsonFlag bool

func init() {
	waitCmd.Flags().IntVar(&waitTimeoutFlag, "timeout", 30, "Timeout in seconds")
	waitCmd.Flags().BoolVar(&waitJsonFlag, "json", false, "Output as JSON")
}

func runWait(cmd *cobra.Command, args []string) error {
	name := args[0]
	pattern := args[1]

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	timeout := time.Duration(waitTimeoutFlag) * time.Second
	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	var lastOutput string
	var lastPos int

	for time.Now().Before(deadline) {
		output, pos, err := client.Read(name, "all")
		if err != nil {
			return err
		}

		if re.MatchString(output) {
			if waitJsonFlag {
				out := map[string]interface{}{
					"matched":  true,
					"output":   output,
					"position": pos,
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("Pattern matched\n")
			}
			return nil
		}

		lastOutput = output
		lastPos = pos
		time.Sleep(pollInterval)
	}

	if waitJsonFlag {
		out := map[string]interface{}{
			"matched":  false,
			"output":   lastOutput,
			"position": lastPos,
			"error":    "timeout",
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	}

	return fmt.Errorf("timeout waiting for pattern %q", pattern)
}
