package cmd

import (
	"fmt"

	"github.com/schovi/shelli/internal/daemon"
	"github.com/schovi/shelli/internal/escape"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <input> [input...]",
	Short: "Send raw input to a session",
	Long: `Send raw input to a session. Low-level command for precise control.

Each argument is sent as a separate write to the PTY.
Escape sequences are always interpreted. No newline is added automatically.

Escape sequences:
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
  shelli send session "ls -la\n"      # command with newline
  shelli send session "hello" "\r"    # TUI: type "hello", then Enter (separate writes)
  shelli send session "hello\r"       # TUI: same but single write (may not work for all TUIs)
  shelli send session "\x03"          # send Ctrl+C
  shelli send session "\x04"          # send Ctrl+D (EOF)
  shelli send session "y"             # send 'y' without newline
  shelli send session "path\\nname"   # literal backslash-n (escaped)`,
	Args: cobra.MinimumNArgs(2),
	RunE: runSend,
}

func runSend(cmd *cobra.Command, args []string) error {
	name := args[0]
	inputs := args[1:]

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	totalBytes := 0
	for _, input := range inputs {
		interpreted, err := escape.Interpret(input)
		if err != nil {
			return fmt.Errorf("escape sequence error: %w", err)
		}

		if err := client.Send(name, interpreted, false); err != nil {
			return err
		}
		totalBytes += len(interpreted)
	}

	if len(inputs) == 1 {
		fmt.Printf("Sent to %q (%d bytes)\n", name, totalBytes)
	} else {
		fmt.Printf("Sent %d inputs to %q (%d bytes total)\n", len(inputs), name, totalBytes)
	}
	return nil
}
