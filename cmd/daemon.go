package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/schovi/shelli/internal/mcp"
	"github.com/spf13/cobra"
)

var (
	daemonMaxOutputFlag   string
	daemonMCPFlag         bool
	daemonDataDirFlag     string
	daemonMemoryBackend   bool
	daemonStoppedTTLFlag  string
	daemonLogFileFlag     string
)

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the shelli daemon (internal)",
	Hidden: true,
	RunE:   runDaemon,
}

func init() {
	daemonCmd.Flags().StringVar(&daemonMaxOutputFlag, "max-output", "10MB",
		"Maximum output buffer size per session for memory backend (e.g., 10MB, 1GB)")
	daemonCmd.Flags().BoolVar(&daemonMCPFlag, "mcp", false,
		"Run as MCP server (JSON-RPC over stdio)")
	daemonCmd.Flags().StringVar(&daemonDataDirFlag, "data-dir", "",
		"Directory for session output files (default: /tmp/shelli-{uid}/data)")
	daemonCmd.Flags().BoolVar(&daemonMemoryBackend, "memory-backend", false,
		"Use in-memory storage instead of file-based (no persistence)")
	daemonCmd.Flags().StringVar(&daemonStoppedTTLFlag, "stopped-ttl", "",
		"Auto-cleanup stopped sessions after duration (e.g., 5m, 1h, 24h)")
	daemonCmd.Flags().StringVar(&daemonLogFileFlag, "log-file", "",
		"Write daemon logs to file (default: discard)")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	if daemonMCPFlag {
		return runMCPServer()
	}

	if daemonLogFileFlag != "" {
		f, err := os.OpenFile(daemonLogFileFlag, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer f.Close()
		log.SetOutput(f)
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		os.Stderr = f
		log.Println("daemon starting")
	}

	var opts []daemon.ServerOption

	if daemonDataDirFlag == "" {
		runtimeDir, err := daemon.RuntimeDir()
		if err != nil {
			return fmt.Errorf("get runtime dir: %w", err)
		}
		daemonDataDirFlag = filepath.Join(runtimeDir, "data")
	}

	if daemonMemoryBackend {
		maxSize, err := parseSize(daemonMaxOutputFlag)
		if err != nil {
			return fmt.Errorf("invalid --max-output: %w", err)
		}
		opts = append(opts, daemon.WithStorage(daemon.NewMemoryStorage(maxSize)))
	} else {
		fileStorage, err := daemon.NewFileStorage(daemonDataDirFlag)
		if err != nil {
			return fmt.Errorf("create file storage: %w", err)
		}
		opts = append(opts, daemon.WithStorage(fileStorage))
	}

	if daemonStoppedTTLFlag != "" {
		ttl, err := time.ParseDuration(daemonStoppedTTLFlag)
		if err != nil {
			return fmt.Errorf("invalid --stopped-ttl: %w", err)
		}
		opts = append(opts, daemon.WithStoppedTTL(ttl))
	}

	server, err := daemon.NewServer(opts...)
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

func runMCPServer() error {
	tools := mcp.NewToolRegistry()
	server := mcp.NewServer(tools, version)
	return server.Run()
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
