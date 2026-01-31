package cmd

import (
	"fmt"
	"strings"

	"github.com/schovi/ishell/internal/daemon"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <input>",
	Short: "Send input to a session (newline by default)",
	Long: `Send input to a session. Appends newline by default.
Use --raw to send without newline (for control characters like Ctrl+C).`,
	Args: cobra.MinimumNArgs(2),
	RunE: runSend,
}

var sendRawFlag bool

func init() {
	sendCmd.Flags().BoolVar(&sendRawFlag, "raw", false, "Send without newline (for control chars)")
}

func runSend(cmd *cobra.Command, args []string) error {
	name := args[0]
	input := strings.Join(args[1:], " ")

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	newline := !sendRawFlag
	if err := client.Send(name, input, newline); err != nil {
		return err
	}

	fmt.Printf("Sent to %q\n", name)
	return nil
}
