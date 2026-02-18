package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/ansi"
	"github.com/schovi/shelli/internal/daemon"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <name> <pattern>",
	Short: "Search session output for patterns",
	Long: `Search session output buffer for regex patterns with context.

Returns matching lines with optional context lines before and after.`,
	Args: cobra.ExactArgs(2),
	RunE: runSearch,
}

var (
	searchBeforeFlag    int
	searchAfterFlag     int
	searchAroundFlag    int
	searchIgnoreCaseFlag bool
	searchStripAnsiFlag bool
	searchJsonFlag      bool
)

func init() {
	searchCmd.Flags().IntVar(&searchBeforeFlag, "before", 0, "Lines of context before each match")
	searchCmd.Flags().IntVar(&searchAfterFlag, "after", 0, "Lines of context after each match")
	searchCmd.Flags().IntVar(&searchAroundFlag, "around", 0, "Lines of context before AND after (shorthand for --before N --after N)")
	searchCmd.Flags().BoolVar(&searchIgnoreCaseFlag, "ignore-case", false, "Case-insensitive search")
	searchCmd.Flags().BoolVar(&searchStripAnsiFlag, "strip-ansi", false, "Strip ANSI escape codes before searching")
	searchCmd.Flags().BoolVar(&searchJsonFlag, "json", false, "Output as JSON")
}

func runSearch(cmd *cobra.Command, args []string) error {
	name := args[0]
	pattern := args[1]

	if searchAroundFlag > 0 && (searchBeforeFlag > 0 || searchAfterFlag > 0) {
		return fmt.Errorf("--around is mutually exclusive with --before/--after")
	}

	before := searchBeforeFlag
	after := searchAfterFlag
	if searchAroundFlag > 0 {
		before = searchAroundFlag
		after = searchAroundFlag
	}

	if before < 0 || after < 0 {
		return fmt.Errorf("--before, --after, and --around must be non-negative")
	}

	client := daemon.NewClient()
	if err := client.EnsureDaemon(); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	resp, err := client.Search(daemon.SearchRequest{
		Name:       name,
		Pattern:    pattern,
		Before:     before,
		After:      after,
		IgnoreCase: searchIgnoreCaseFlag,
		StripANSI:  searchStripAnsiFlag,
	})
	if err != nil {
		return err
	}

	if searchJsonFlag {
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if len(resp.Matches) == 0 {
		fmt.Println("No matches found.")
		return nil
	}

	for i, match := range resp.Matches {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("--- Match at line %d ---\n", match.LineNumber)

		startLine := match.LineNumber - len(match.Before)
		for j, line := range match.Before {
			display := line
			if searchStripAnsiFlag {
				display = ansi.Strip(line)
			}
			fmt.Printf("%4d: %s\n", startLine+j, display)
		}

		display := match.Line
		if searchStripAnsiFlag {
			display = ansi.Strip(match.Line)
		}
		fmt.Printf(">%3d: %s\n", match.LineNumber, display)

		for j, line := range match.After {
			display := line
			if searchStripAnsiFlag {
				display = ansi.Strip(line)
			}
			fmt.Printf("%4d: %s\n", match.LineNumber+1+j, display)
		}
	}

	return nil
}
