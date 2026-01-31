package cmd

import (
	"fmt"
	"strings"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/schovi/shelli/internal/escape"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <input>",
	Short: "Send input to a session (newline by default)",
	Long: `Send input to a session. Appends newline by default.

Use --raw to send without newline and with escape sequence interpretation.
This is useful for control characters like Ctrl+C.

Escape sequences (with --raw):
  \x00-\xFF  Hex byte (e.g., \x03 for Ctrl+C)
  \n         Newline (LF)
  \r         Carriage return (CR)
  \t         Tab
  \e         Escape (ASCII 27)
  \\         Literal backslash
  \0         Null byte

Common control characters:
  \x03  Ctrl+C (interrupt)
  \x04  Ctrl+D (EOF)
  \x1a  Ctrl+Z (suspend)
  \x1c  Ctrl+\ (quit)
  \x0c  Ctrl+L (clear screen)

Examples:
  shelli send session "ls -la"           # normal command
  shelli send session "\x03" --raw       # send Ctrl+C
  shelli send session "\x04" --raw       # send Ctrl+D (EOF)
  shelli send session "y" --raw          # send 'y' without newline
  shelli send session "user\tname" --raw # send with tab`,
	Args: cobra.MinimumNArgs(2),
	RunE: runSend,
}

var sendRawFlag bool

func init() {
	sendCmd.Flags().BoolVar(&sendRawFlag, "raw", false, "No newline, interpret escape sequences")
}

func runSend(cmd *cobra.Command, args []string) error {
	name := args[0]
	input := strings.Join(args[1:], " ")

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	if sendRawFlag {
		// Interpret escape sequences
		interpreted, err := escape.Interpret(input)
		if err != nil {
			return fmt.Errorf("escape sequence error: %w", err)
		}
		input = interpreted
	}

	newline := !sendRawFlag
	if err := client.Send(name, input, newline); err != nil {
		return err
	}

	if sendRawFlag {
		fmt.Printf("Sent raw to %q (%d bytes)\n", name, len(input))
	} else {
		fmt.Printf("Sent to %q\n", name)
	}
	return nil
}
