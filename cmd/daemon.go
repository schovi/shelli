package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var daemonMaxOutputFlag string

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the shelli daemon (internal)",
	Hidden: true,
	RunE:   runDaemon,
}

func init() {
	daemonCmd.Flags().StringVar(&daemonMaxOutputFlag, "max-output", "10MB",
		"Maximum output buffer size per session (e.g., 10MB, 1GB)")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	maxSize, err := parseSize(daemonMaxOutputFlag)
	if err != nil {
		return fmt.Errorf("invalid --max-output: %w", err)
	}

	server, err := daemon.NewServer(daemon.WithMaxOutputSize(maxSize))
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("Shutting down daemon...")
		server.Shutdown()
		os.Exit(0)
	}()

	return server.Start()
}

func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(B|KB|MB|GB)?$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid format: %s", s)
	}

	val, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	if unit == "" {
		unit = "B"
	}

	multiplier := 1.0
	switch unit {
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	}

	return int(val * multiplier), nil
}
