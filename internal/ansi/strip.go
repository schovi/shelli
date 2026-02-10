package ansi

import (
	"regexp"
	"strconv"
	"strings"
)

var ansiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`),             // CSI sequences (colors, cursor, etc)
	regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`), // OSC sequences
	regexp.MustCompile(`\x1b[()][AB012]`),                    // Character set selection
	regexp.MustCompile(`\x1b[=>]`),                           // Keypad modes
	regexp.MustCompile(`\x1b\[\?[0-9;]*[hlsr]`),             // DEC private modes
	regexp.MustCompile(`\x1b[A-Za-z]`),                       // Simple ESC+letter sequences (RI, IND, NEL, RIS, etc.)
	regexp.MustCompile(`\r`),                                  // Carriage returns
}

var cursorPosPattern = regexp.MustCompile(`\x1b\[(\d+);(\d+)([HF])`)

const (
	maxGridCols = 500
	maxGridRows = 500
)

func convertCursorPositioning(s string) string {
	if !cursorPosPattern.MatchString(s) {
		return s
	}

	// Pre-scan to determine grid dimensions
	maxRow, maxCol := 0, 0
	for _, match := range cursorPosPattern.FindAllStringSubmatch(s, -1) {
		row, _ := strconv.Atoi(match[1])
		col, _ := strconv.Atoi(match[2])
		if row > maxRow {
			maxRow = row
		}
		if col > maxCol {
			maxCol = col
		}
	}

	// Estimate extra cols needed for text after last cursor position on each row
	maxCol += 80
	if maxCol > maxGridCols {
		maxCol = maxGridCols
	}
	if maxRow > maxGridRows {
		maxRow = maxGridRows
	}

	// Allocate grid filled with spaces
	grid := make([][]byte, maxRow)
	for i := range grid {
		grid[i] = make([]byte, maxCol)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	curRow, curCol := 0, 0

	i := 0
	for i < len(s) {
		if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '[' {
			// Try to match cursor position sequence
			remaining := s[i:]
			loc := cursorPosPattern.FindStringSubmatchIndex(remaining)
			if loc != nil && loc[0] == 0 {
				row, _ := strconv.Atoi(remaining[loc[2]:loc[3]])
				col, _ := strconv.Atoi(remaining[loc[4]:loc[5]])
				// Convert from 1-based to 0-based
				curRow = row - 1
				curCol = col - 1
				if curRow < 0 {
					curRow = 0
				}
				if curCol < 0 {
					curCol = 0
				}
				i += loc[1]
				continue
			}

			// Skip other ESC[ sequences
			j := i + 2
			for j < len(s) && !isCSITerminator(s[j]) {
				j++
			}
			if j < len(s) {
				j++ // skip terminator
			}
			i = j
			continue
		}

		if s[i] == 0x1B {
			// 3-byte ESC sequences: ESC(X, ESC)X, ESC#X (character set, DEC line drawing)
			if i+2 < len(s) && (s[i+1] == '(' || s[i+1] == ')' || s[i+1] == '#') {
				i += 3
				continue
			}
			// Skip other ESC sequences (ESC + one char)
			i += 2
			if i > len(s) {
				i = len(s)
			}
			continue
		}

		if s[i] == '\n' {
			curRow++
			curCol = 0
			i++
			continue
		}

		if s[i] == '\r' {
			curCol = 0
			i++
			continue
		}

		// Printable character: write to grid
		if curRow >= 0 && curRow < maxRow && curCol >= 0 && curCol < maxCol {
			grid[curRow][curCol] = s[i]
		}
		curCol++
		i++
	}

	// Join grid rows, right-trim trailing spaces, remove trailing empty rows
	var b strings.Builder
	lastNonEmptyRow := -1
	for r := 0; r < maxRow; r++ {
		trimmed := strings.TrimRight(string(grid[r]), " ")
		if trimmed != "" {
			lastNonEmptyRow = r
		}
	}

	for r := 0; r <= lastNonEmptyRow; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(string(grid[r]), " "))
	}

	return b.String()
}

func isCSITerminator(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '@' || c == '`'
}

func Strip(s string) string {
	result := convertCursorPositioning(s)
	for _, re := range ansiPatterns {
		result = re.ReplaceAllString(result, "")
	}
	return result
}
