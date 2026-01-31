package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the ishell daemon (internal)",
	Hidden: true,
	RunE:   runDaemon,
}

func runDaemon(cmd *cobra.Command, args []string) error {
	server, err := daemon.NewServer()
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("Shutting down daemon...")
		os.Exit(0)
	}()

	return server.Start()
}
