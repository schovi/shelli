package escape

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Interpret processes escape sequences in a string.
// Supported sequences:
//
//	\x00-\xFF  - Hex byte (e.g., \x03 for Ctrl+C)
//	\n         - Newline (LF)
//	\r         - Carriage return (CR)
//	\t         - Tab
//	\e         - Escape (ASCII 27)
//	\\         - Literal backslash
//
// Unrecognized escape sequences pass through literally (backslash is dropped).
// For example, \! becomes !, \? becomes ?.
//
// Common control characters:
//
//	\x03 - Ctrl+C (interrupt)
//	\x04 - Ctrl+D (EOF)
//	\x1a - Ctrl+Z (suspend)
//	\x1c - Ctrl+\ (quit)
//	\x0c - Ctrl+L (clear screen)
func Interpret(s string) (string, error) {
	var result strings.Builder
	result.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			result.WriteByte(s[i])
			i++
			continue
		}

		// Handle escape sequence
		if i+1 >= len(s) {
			return "", fmt.Errorf("incomplete escape sequence at end of string")
		}

		switch s[i+1] {
		case 'x':
			// Hex escape: \xNN
			if i+3 >= len(s) {
				return "", fmt.Errorf("incomplete hex escape sequence at position %d", i)
			}
			hex := s[i+2 : i+4]
			val, err := strconv.ParseUint(hex, 16, 8)
			if err != nil {
				return "", fmt.Errorf("invalid hex escape \\x%s at position %d", hex, i)
			}
			result.WriteByte(byte(val))
			i += 4

		case 'n':
			result.WriteByte('\n')
			i += 2

		case 'r':
			result.WriteByte('\r')
			i += 2

		case 't':
			result.WriteByte('\t')
			i += 2

		case 'e':
			result.WriteByte(0x1b) // ESC
			i += 2

		case '\\':
			result.WriteByte('\\')
			i += 2

		case '0':
			result.WriteByte(0x00) // NUL
			i += 2

		default:
			r, size := utf8.DecodeRuneInString(s[i+1:])
			result.WriteRune(r)
			i += 1 + size
		}
	}

	return result.String(), nil
}
