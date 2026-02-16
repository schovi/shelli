package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	readFollowFlag    bool
	readFollowMsFlag  int
	readSnapshotFlag  bool
	readCursorFlag    string
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
	readCmd.Flags().BoolVarP(&readFollowFlag, "follow", "f", false, "Follow output continuously (like tail -f)")
	readCmd.Flags().IntVar(&readFollowMsFlag, "follow-ms", 100, "Poll interval for --follow in milliseconds")
	readCmd.Flags().BoolVar(&readSnapshotFlag, "snapshot", false, "Force TUI redraw and read clean frame (TUI sessions only)")
	readCmd.Flags().StringVar(&readCursorFlag, "cursor", "", "Named cursor for per-consumer read tracking")
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

	if readSnapshotFlag {
		if readFollowFlag || readAllFlag || hasWait {
			return fmt.Errorf("--snapshot cannot be combined with --follow, --all, or --wait")
		}
		return runReadSnapshot(name)
	}

	if readFollowFlag {
		if readAllFlag || readHeadFlag > 0 || readTailFlag > 0 || blocking || readJsonFlag {
			return fmt.Errorf("--follow cannot be combined with --all, --head, --tail, --wait, --settle, or --json")
		}
		return runReadFollow(name)
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
		_, startPos, readErr := client.Read(name, "all", 0, 0)
		if readErr != nil {
			return readErr
		}

		output, pos, err = wait.ForOutput(
			func() (string, int, error) { return client.Read(name, "all", 0, 0) },
			wait.Config{
				Pattern:       readWaitFlag,
				SettleMs:      readSettleFlag,
				TimeoutSec:    readTimeoutFlag,
				StartPosition: startPos,
				SizeFunc:      func() (int, error) { return client.Size(name) },
			},
		)
		if err == nil {
			if headLines > 0 || tailLines > 0 {
				output = daemon.LimitLines(output, headLines, tailLines)
			}
			if readCursorFlag != "" {
				client.ReadWithCursor(name, "new", readCursorFlag, 0, 0)
			} else {
				client.Read(name, "new", 0, 0)
			}
		}
	} else {
		mode := daemon.ReadModeNew
		if readAllFlag || readHeadFlag > 0 || readTailFlag > 0 {
			mode = daemon.ReadModeAll
		}
		if readCursorFlag != "" {
			output, pos, err = client.ReadWithCursor(name, mode, readCursorFlag, headLines, tailLines)
		} else {
			output, pos, err = client.Read(name, mode, headLines, tailLines)
		}
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

func runReadSnapshot(name string) error {
	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	settleMs := readSettleFlag
	output, pos, err := client.Snapshot(name, settleMs, readTimeoutFlag, readHeadFlag, readTailFlag)
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

func runReadFollow(name string) error {
	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if readFollowMsFlag <= 0 {
		readFollowMsFlag = 100
	}
	pollInterval := time.Duration(readFollowMsFlag) * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			output, _, err := client.Read(name, daemon.ReadModeNew, 0, 0)
			if err != nil {
				return err
			}
			if output != "" {
				if readStripAnsiFlag {
					output = ansi.Strip(output)
				}
				fmt.Print(output)
			}
		}
	}
}
