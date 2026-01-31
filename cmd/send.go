package cmd

import (
	"fmt"
	"strings"

	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <input>",
	Short: "Send input to a session (no newline)",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runSend,
}

var sendlineCmd = &cobra.Command{
	Use:   "sendline <name> <input>",
	Short: "Send input to a session with newline",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runSendline,
}

func runSend(cmd *cobra.Command, args []string) error {
	return doSend(args[0], strings.Join(args[1:], " "), false)
}

func runSendline(cmd *cobra.Command, args []string) error {
	return doSend(args[0], strings.Join(args[1:], " "), true)
}

func doSend(name, input string, newline bool) error {
	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if err := client.Send(name, input, newline); err != nil {
		return err
	}

	fmt.Printf("Sent to %q\n", name)
	return nil
}
