package ansi

import "testing"

func TestStrip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text unchanged",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "strip color codes",
			input:    "\x1b[31mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "strip bold color",
			input:    "\x1b[1;32mgreen bold\x1b[0m",
			expected: "green bold",
		},
		{
			name:     "strip cursor movement",
			input:    "\x1b[2Aup two lines",
			expected: "up two lines",
		},
		{
			name:     "strip clear screen",
			input:    "\x1b[2Jcleared",
			expected: "cleared",
		},
		{
			name:     "strip OSC sequences",
			input:    "\x1b]0;window title\x07text",
			expected: "text",
		},
		{
			name:     "strip carriage return",
			input:    "line1\rline2",
			expected: "line1line2",
		},
		{
			name:     "strip DEC private mode",
			input:    "\x1b[?25hvisible cursor",
			expected: "visible cursor",
		},
		{
			name:     "strip character set selection",
			input:    "\x1b(Btext",
			expected: "text",
		},
		{
			name:     "complex mixed sequences",
			input:    "\x1b[1;34m\x1b[2Juser@host:\x1b[0m$ ls\r\n",
			expected: "user@host:$ ls\n",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only escape sequences",
			input:    "\x1b[31m\x1b[0m\r",
			expected: "",
		},
		{
			name:     "strip reverse index (ESC M)",
			input:    "\x1bMline content",
			expected: "line content",
		},
		{
			name:     "strip index / line feed (ESC D)",
			input:    "\x1bDline content",
			expected: "line content",
		},
		{
			name:     "strip full reset (ESC c)",
			input:    "\x1bcline content",
			expected: "line content",
		},
		{
			name:     "mixed ESC+letter with CSI",
			input:    "\x1bMline content\x1b[31mred\x1b[0m",
			expected: "line contentred",
		},
		{
			name:     "cursor positioning different rows produce newlines",
			input:    "\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3",
			expected: "row1\nrow2\nrow3",
		},
		{
			name:     "cursor positioning same row different columns no newline",
			input:    "\x1b[1;1Hfirst\x1b[1;10Hsecond",
			expected: "firstsecond",
		},
		{
			name:     "cursor positioning interleaved with color codes",
			input:    "\x1b[1;1H\x1b[31mred\x1b[0m\x1b[2;1H\x1b[32mgreen\x1b[0m",
			expected: "red\ngreen",
		},
		{
			name:     "cursor positioning F variant",
			input:    "\x1b[1;1Frow1\x1b[2;1Frow2",
			expected: "row1\nrow2",
		},
		{
			name:     "no cursor positioning unchanged",
			input:    "plain text\nwith newlines",
			expected: "plain text\nwith newlines",
		},
		{
			name:     "cursor positioning row jump backward",
			input:    "\x1b[5;1Hrow5\x1b[1;1Hrow1",
			expected: "row5\nrow1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Strip(tt.input)
			if got != tt.expected {
				t.Errorf("Strip(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
