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

func convertCursorPositioning(s string) string {
	matches := cursorPosPattern.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + len(matches))

	lastRow := -1
	prev := 0

	for _, loc := range cursorPosPattern.FindAllStringSubmatchIndex(s, -1) {
		// loc[0]:loc[1] = full match, loc[2]:loc[3] = row, loc[4]:loc[5] = col
		b.WriteString(s[prev:loc[0]])

		row, _ := strconv.Atoi(s[loc[2]:loc[3]])
		if lastRow != -1 && row != lastRow {
			b.WriteByte('\n')
		}
		lastRow = row

		prev = loc[1]
	}

	b.WriteString(s[prev:])
	return b.String()
}

func Strip(s string) string {
	result := convertCursorPositioning(s)
	for _, re := range ansiPatterns {
		result = re.ReplaceAllString(result, "")
	}
	return result
}
