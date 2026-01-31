package ansi

import "regexp"

var ansiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`),      // CSI sequences (colors, cursor, etc)
	regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`), // OSC sequences
	regexp.MustCompile(`\x1b[()][AB012]`),            // Character set selection
	regexp.MustCompile(`\x1b[=>]`),                   // Keypad modes
	regexp.MustCompile(`\x1b\[\?[0-9;]*[hlsr]`),      // DEC private modes
	regexp.MustCompile(`\r`),                         // Carriage returns
}

func Strip(s string) string {
	result := s
	for _, re := range ansiPatterns {
		result = re.ReplaceAllString(result, "")
	}
	return result
}
