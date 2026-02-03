package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var infoJsonFlag bool

func init() {
	infoCmd.Flags().BoolVar(&infoJsonFlag, "json", false, "Output as JSON")
}

var infoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show detailed session information",
	Long:  `Display detailed information about a session including state, PID, command, buffer size, and terminal dimensions.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runInfo,
}

func runInfo(cmd *cobra.Command, args []string) error {
	name := args[0]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	info, err := client.Info(name)
	if err != nil {
		return err
	}

	if infoJsonFlag {
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Session: %s\n", info.Name)
		fmt.Printf("State:   %s\n", info.State)
		fmt.Printf("PID:     %d\n", info.PID)
		fmt.Printf("Command: %s\n", info.Command)
		fmt.Printf("Created: %s\n", info.CreatedAt)
		if info.StoppedAt != "" {
			fmt.Printf("Stopped: %s\n", info.StoppedAt)
		}
		if info.Uptime > 0 {
			fmt.Printf("Uptime:  %s\n", formatDuration(info.Uptime))
		}
		fmt.Printf("Buffer:  %d bytes\n", info.BytesBuffered)
		fmt.Printf("ReadPos: %d\n", info.ReadPosition)
		fmt.Printf("Size:    %dx%d\n", info.Cols, info.Rows)
	}
	return nil
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", seconds)
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm%ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
