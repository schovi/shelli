package vterm

import (
	"io"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/vt"
)

var ansiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z~]`),            // CSI sequences (colors, cursor, etc)
	regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`), // OSC sequences
	regexp.MustCompile(`\x1b[()][AB012]`),                    // Character set selection
	regexp.MustCompile(`\x1b[=>]`),                           // Keypad modes
	regexp.MustCompile(`\x1b\[\?[0-9;]*[hlsr]`),             // DEC private modes
	regexp.MustCompile(`\x1b[A-Za-z]`),                       // Simple ESC+letter sequences
	regexp.MustCompile(`\r`),                                  // Carriage returns
}

var cursorAnyPattern = regexp.MustCompile(`\x1b\[\d*;?\d*[HFfGdABCD]`)

// loneNewline matches \n not preceded by \r (standalone line feeds).
var loneNewline = regexp.MustCompile(`(?:^|[^\r])\n`)

// Strip removes ANSI escape sequences from s. When cursor positioning sequences
// are detected, a temporary VT emulator is used for correct rendering. Otherwise,
// a fast regex-based strip is used.
func Strip(s string, cols int) string {
	if s == "" {
		return ""
	}
	if cols <= 0 {
		cols = 200
	}

	if !cursorAnyPattern.MatchString(s) {
		result := s
		for _, re := range ansiPatterns {
			result = re.ReplaceAllString(result, "")
		}
		return result
	}

	rows := strings.Count(s, "\n") + 100
	if rows > 5000 {
		rows = 5000
	}

	// VT emulator treats \n as line-feed-only (no carriage return).
	// Real terminals with ONLCR convert \n to \r\n. Pre-process to match.
	input := normalizeNewlines(s)

	emu := vt.NewEmulator(cols, rows)

	// The emulator writes terminal query responses (DA1/DA2, etc.) to its internal
	// pipe. Without a goroutine draining that pipe, writes block when the buffer fills,
	// causing a deadlock. Drain and discard all responses.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		io.Copy(io.Discard, emu) //nolint:errcheck
	}()

	emu.WriteString(input)
	result := emu.String()
	if pw, ok := emu.InputPipe().(io.Closer); ok {
		pw.Close()
	}
	<-drainDone
	emu.Close()

	// VT emulator uses \r\n line endings; normalize to \n
	result = strings.ReplaceAll(result, "\r\n", "\n")
	result = strings.ReplaceAll(result, "\r", "")

	return trimTrailingEmptyLines(result)
}

// StripDefault strips ANSI with a default column width of 200.
func StripDefault(s string) string {
	return Strip(s, 200)
}

// normalizeNewlines converts standalone \n (not preceded by \r) to \r\n,
// matching what a real terminal driver does with ONLCR.
func normalizeNewlines(s string) string {
	// Fast path: no standalone \n
	if !loneNewline.MatchString(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 32)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteByte('\r')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
