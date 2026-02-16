package ansi

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ansiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z~]`),            // CSI sequences (colors, cursor, etc)
	regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`), // OSC sequences
	regexp.MustCompile(`\x1b[()][AB012]`),                    // Character set selection
	regexp.MustCompile(`\x1b[=>]`),                           // Keypad modes
	regexp.MustCompile(`\x1b\[\?[0-9;]*[hlsr]`),             // DEC private modes
	regexp.MustCompile(`\x1b[A-Za-z]`),                       // Simple ESC+letter sequences (RI, IND, NEL, RIS, etc.)
	regexp.MustCompile(`\r`),                                  // Carriage returns
}

var cursorPosPattern = regexp.MustCompile(`\x1b\[(\d+);(\d+)([HFf])`)
var cursorAnyPattern = regexp.MustCompile(`\x1b\[\d*;?\d*[HFfGdABCD]`)

const (
	maxGridCols = 500
	maxGridRows = 500
)

// DEC Special Graphics character set mapping (ESC(0 activates, ESC(B deactivates)
var decSpecialGraphics = map[byte]rune{
	'`': '◆', 'a': '▒', 'b': '\t', 'c': '\f', 'd': '\r', 'e': '\n',
	'f': '°', 'g': '±', 'h': '\n', 'i': '\v', 'j': '┘', 'k': '┐',
	'l': '┌', 'm': '└', 'n': '┼', 'o': '⎺', 'p': '⎻',
	'q': '─', 'r': '⎼', 's': '⎽', 't': '├', 'u': '┤',
	'v': '┴', 'w': '┬', 'x': '│', 'y': '≤', 'z': '≥',
	'{': 'π', '|': '≠', '}': '£', '~': '·',
}

func convertCursorPositioning(s string) string {
	if !cursorAnyPattern.MatchString(s) {
		return s
	}

	// Pre-scan to determine grid dimensions from all cursor positioning variants
	maxRow, maxCol := 0, 0
	hasRelativeMovement := false
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
	// Also scan single-arg sequences: ESC[nH (row), ESC[nG (col), ESC[nd (row), ESC[nA/B/C/D (relative)
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1B || i+1 >= len(s) || s[i+1] != '[' {
			continue
		}
		j := i + 2
		num := 0
		hasNum := false
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			num = num*10 + int(s[j]-'0')
			hasNum = true
			j++
		}
		if j >= len(s) {
			break
		}
		if s[j] == ';' {
			continue // row;col form, already handled by cursorPosPattern
		}
		if !hasNum {
			num = 1 // defaults to 1
		}
		switch s[j] {
		case 'H', 'F', 'f':
			if num > maxRow {
				maxRow = num
			}
		case 'd':
			if num > maxRow {
				maxRow = num
			}
		case 'G':
			if num > maxCol {
				maxCol = num
			}
		case 'A', 'B', 'C', 'D':
			hasRelativeMovement = true
		}
	}

	// Relative movement can reach beyond absolute positions, ensure minimum grid size
	if hasRelativeMovement {
		if maxRow < 50 {
			maxRow = 50
		}
		if maxCol < 120 {
			maxCol = 120
		}
	}

	// Ensure grid accommodates newline-separated content (handles less/bat where
	// only ESC[H is used for positioning but content flows with newlines)
	newlineCount := strings.Count(s, "\n")
	if newlineCount+1 > maxRow {
		maxRow = newlineCount + 1
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
	grid := make([][]rune, maxRow)
	for i := range grid {
		grid[i] = make([]rune, maxCol)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	curRow, curCol := 0, 0
	useGraphicsCharset := false

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
				newRow := row - 1
				newCol := col - 1
				if newRow < 0 {
					newRow = 0
				}
				if newCol < 0 {
					newCol = 0
				}
				if newRow == 0 && newCol == 0 && curRow >= 10 && hasPrintableAhead(s, i+loc[1], 100) {
					for r := range grid {
						for c := range grid[r] {
							grid[r][c] = ' '
						}
					}
				}
				curRow = newRow
				curCol = newCol
				i += loc[1]
				continue
			}

			// Try single-arg cursor sequences: ESC[nH, ESC[H, ESC[nG, ESC[nd
			if row, col, end, ok := parseSingleArgCursor(s, i, curRow, curCol); ok {
				if row == 0 && col == 0 && curRow >= 10 && hasPrintableAhead(s, end, 100) {
					for r := range grid {
						for c := range grid[r] {
							grid[r][c] = ' '
						}
					}
				}
				curRow = row
				curCol = col
				i = end
				continue
			}

			// Try relative cursor movement: ESC[nA (up), ESC[nB (down), ESC[nC (right), ESC[nD (left)
			if newRow, newCol, end, ok := parseRelativeCursor(s, i, curRow, curCol); ok {
				curRow = newRow
				curCol = newCol
				if curRow < 0 {
					curRow = 0
				}
				if curCol < 0 {
					curCol = 0
				}
				i = end
				continue
			}

			// Erase line: ESC[K, ESC[0K, ESC[1K, ESC[2K
			if mode, end, ok := parseEraseLine(s, i); ok {
				if curRow >= 0 && curRow < maxRow {
					switch mode {
					case 0: // cursor to end of line
						for c := curCol; c < maxCol; c++ {
							grid[curRow][c] = ' '
						}
					case 1: // start of line to cursor
						for c := 0; c <= curCol && c < maxCol; c++ {
							grid[curRow][c] = ' '
						}
					case 2: // entire line
						for c := 0; c < maxCol; c++ {
							grid[curRow][c] = ' '
						}
					}
				}
				i = end
				continue
			}

			// Erase display: ESC[J, ESC[0J, ESC[1J, ESC[2J
			if mode, end, ok := parseEraseDisplay(s, i); ok {
				switch mode {
				case 0: // cursor to end of display
					if curRow >= 0 && curRow < maxRow {
						for c := curCol; c < maxCol; c++ {
							grid[curRow][c] = ' '
						}
						for r := curRow + 1; r < maxRow; r++ {
							for c := 0; c < maxCol; c++ {
								grid[r][c] = ' '
							}
						}
					}
				case 1: // start of display to cursor
					for r := 0; r < curRow && r < maxRow; r++ {
						for c := 0; c < maxCol; c++ {
							grid[r][c] = ' '
						}
					}
					if curRow >= 0 && curRow < maxRow {
						for c := 0; c <= curCol && c < maxCol; c++ {
							grid[curRow][c] = ' '
						}
					}
				case 2: // entire display
					for r := range grid {
						for c := range grid[r] {
							grid[r][c] = ' '
						}
					}
				}
				i = end
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
				if s[i+1] == '(' {
					useGraphicsCharset = s[i+2] == '0'
				}
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

		// Printable character: decode full rune and write to grid
		r, size := utf8.DecodeRuneInString(s[i:])
		if useGraphicsCharset && size == 1 {
			if mapped, ok := decSpecialGraphics[s[i]]; ok {
				r = mapped
			}
		}
		if curRow >= 0 && curRow < maxRow && curCol >= 0 && curCol < maxCol {
			grid[curRow][curCol] = r
		}
		curCol++
		i += size
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

// parseSingleArgCursor parses ESC[nH (row only), ESC[H (home), ESC[nG (col absolute),
// ESC[nd (row absolute) at position i in s. Returns 0-based row, col, end index, and ok.
// curRow/curCol are the current position (used to keep the unchanged dimension).
func parseSingleArgCursor(s string, i, curRow, curCol int) (row, col, end int, ok bool) {
	if i+1 >= len(s) || s[i] != 0x1B || s[i+1] != '[' {
		return 0, 0, 0, false
	}
	j := i + 2
	num := 0
	hasNum := false
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		num = num*10 + int(s[j]-'0')
		hasNum = true
		j++
	}
	if j >= len(s) {
		return 0, 0, 0, false
	}
	if s[j] == ';' {
		return 0, 0, 0, false // row;col form, handled by cursorPosPattern
	}
	if !hasNum {
		num = 1
	}
	switch s[j] {
	case 'H', 'F', 'f':
		return num - 1, 0, j + 1, true // row only, col defaults to 1 (0-based: 0)
	case 'G':
		return curRow, num - 1, j + 1, true // col absolute, keep row
	case 'd':
		return num - 1, curCol, j + 1, true // row absolute, keep col
	}
	return 0, 0, 0, false
}

// parseRelativeCursor parses ESC[nA (up), ESC[nB (down), ESC[nC (right), ESC[nD (left)
// at position i in s. Returns new 0-based row, col, end index, and ok.
func parseRelativeCursor(s string, i, curRow, curCol int) (row, col, end int, ok bool) {
	if i+1 >= len(s) || s[i] != 0x1B || s[i+1] != '[' {
		return 0, 0, 0, false
	}
	j := i + 2
	num := 0
	hasNum := false
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		num = num*10 + int(s[j]-'0')
		hasNum = true
		j++
	}
	if j >= len(s) {
		return 0, 0, 0, false
	}
	if s[j] == ';' {
		return 0, 0, 0, false
	}
	if !hasNum {
		num = 1
	}
	switch s[j] {
	case 'A':
		return curRow - num, curCol, j + 1, true
	case 'B':
		return curRow + num, curCol, j + 1, true
	case 'C':
		return curRow, curCol + num, j + 1, true
	case 'D':
		return curRow, curCol - num, j + 1, true
	}
	return 0, 0, 0, false
}

// parseEraseLine parses ESC[K, ESC[0K, ESC[1K, ESC[2K at position i.
// Returns mode (0=cursor-to-end, 1=start-to-cursor, 2=full line), end index, and ok.
func parseEraseLine(s string, i int) (mode, end int, ok bool) {
	if i+1 >= len(s) || s[i] != 0x1B || s[i+1] != '[' {
		return 0, 0, false
	}
	j := i + 2
	if j >= len(s) {
		return 0, 0, false
	}
	if s[j] == 'K' {
		return 0, j + 1, true // ESC[K = erase to end (mode 0)
	}
	num := 0
	hasNum := false
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		num = num*10 + int(s[j]-'0')
		hasNum = true
		j++
	}
	if j >= len(s) || s[j] != 'K' || !hasNum {
		return 0, 0, false
	}
	if num < 0 || num > 2 {
		return 0, 0, false
	}
	return num, j + 1, true
}

// parseEraseDisplay parses ESC[J, ESC[0J, ESC[1J, ESC[2J at position i.
// Returns mode (0=cursor-to-end, 1=start-to-cursor, 2=full display), end index, and ok.
func parseEraseDisplay(s string, i int) (mode, end int, ok bool) {
	if i+1 >= len(s) || s[i] != 0x1B || s[i+1] != '[' {
		return 0, 0, false
	}
	j := i + 2
	if j >= len(s) {
		return 0, 0, false
	}
	if s[j] == 'J' {
		return 0, j + 1, true // ESC[J = erase to end (mode 0)
	}
	num := 0
	hasNum := false
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		num = num*10 + int(s[j]-'0')
		hasNum = true
		j++
	}
	if j >= len(s) || s[j] != 'J' || !hasNum {
		return 0, 0, false
	}
	if num < 0 || num > 2 {
		return 0, 0, false
	}
	return num, j + 1, true
}

func isCSITerminator(c byte) bool {
	return c >= 0x40 && c <= 0x7E
}

// hasPrintableAhead checks if printable content follows within maxBytes,
// skipping escape sequences. Distinguishes real redraws (content follows
// cursor home) from cursor parking (only control sequences or end of string).
func hasPrintableAhead(s string, pos, maxBytes int) bool {
	end := pos + maxBytes
	if end > len(s) {
		end = len(s)
	}
	i := pos
	for i < end {
		if s[i] == 0x1B {
			if i+1 < end && s[i+1] == '[' {
				j := i + 2
				for j < end && !isCSITerminator(s[j]) {
					j++
				}
				if j < end {
					j++ // skip terminator
				}
				i = j
				continue
			}
			if i+1 < end && (s[i+1] == '(' || s[i+1] == ')' || s[i+1] == '#') {
				i += 3
				continue
			}
			i += 2
			continue
		}
		if s[i] == '\n' || s[i] == '\r' {
			i++
			continue
		}
		if s[i] >= 0x20 && s[i] != 0x7F {
			return true
		}
		i++
	}
	return false
}

func Strip(s string) string {
	result := convertCursorPositioning(s)
	for _, re := range ansiPatterns {
		result = re.ReplaceAllString(result, "")
	}
	return result
}
