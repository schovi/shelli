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
			name:     "cursor positioning same row different columns",
			input:    "\x1b[1;1Hfirst\x1b[1;10Hsecond",
			expected: "first    second",
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
			expected: "row1\n\n\n\nrow5",
		},
		{
			name:     "overwrite at same position",
			input:    "\x1b[1;1Haaaa\x1b[1;1Hbb",
			expected: "bbaa",
		},
		{
			name:     "grid with gaps between columns",
			input:    "\x1b[1;1Ha\x1b[1;10Hb\x1b[1;20Hc",
			expected: "a        b         c",
		},
		{
			name:     "mixed newlines and cursor positioning",
			input:    "line1\n\x1b[3;1Hline3",
			expected: "line1\n\nline3",
		},
		{
			name:     "btop-style multi-column layout",
			input:    "\x1b[1;1HCPU\x1b[1;40HMEM\x1b[2;1H50%\x1b[2;40H8GB",
			expected: "CPU                                    MEM\n50%                                    8GB",
		},
		{
			name:     "ESC(B with cursor positioning does not leave stray B",
			input:    "\x1b[1;1H\x1b(Bhello\x1b[2;1H\x1b(Bworld",
			expected: "hello\nworld",
		},
		{
			name:     "ESC)0 and ESC#8 with cursor positioning",
			input:    "\x1b[1;1H\x1b)0text\x1b[2;1H\x1b#8more",
			expected: "text\nmore",
		},
		{
			name:     "ESC[nH row only sets row with col 1",
			input:    "\x1b[1Hrow1\x1b[2Hrow2\x1b[3Hrow3",
			expected: "row1\nrow2\nrow3",
		},
		{
			name:     "ESC[H cursor home",
			input:    "\x1b[3;5Hdeep\x1b[Hhome",
			expected: "home\n\n    deep",
		},
		{
			name:     "ESC[nG column absolute",
			input:    "\x1b[1;1Hstart\x1b[10Gmid\x1b[20Gend",
			expected: "start    mid       end",
		},
		{
			name:     "ESC[nd row absolute keeps column",
			input:    "\x1b[1;5Hfirst\x1b[3dthird",
			expected: "    first\n\n         third",
		},
		{
			name:     "btop-style separate row and col positioning",
			input:    "\x1b[1HCPU 50%\x1b[2HMEM 8GB\x1b[3HNET 1Mbps",
			expected: "CPU 50%\nMEM 8GB\nNET 1Mbps",
		},
		// Rune grid: multi-byte characters
		{
			name:     "box-drawing characters in positioned output",
			input:    "\x1b[1;1H┌──┐\x1b[2;1H│  │\x1b[3;1H└──┘",
			expected: "┌──┐\n│  │\n└──┘",
		},
		{
			name:     "emoji in positioned output",
			input:    "\x1b[1;1H🎉hello\x1b[2;1H🚀world",
			expected: "🎉hello\n🚀world",
		},
		// Relative cursor movement
		{
			name:     "cursor down ESC[nB",
			input:    "\x1b[1;1Hline1\x1b[1Bline2",
			expected: "line1\n     line2",
		},
		{
			name:     "cursor right ESC[nC",
			input:    "\x1b[1;1Hstart\x1b[5Cend",
			expected: "start     end",
		},
		{
			name:     "cursor up ESC[nA",
			input:    "\x1b[3;1Hbottom\x1b[2Atop",
			expected: "      top\n\nbottom",
		},
		{
			name:     "cursor left ESC[nD",
			input:    "\x1b[1;10Hright\x1b[8Dleft",
			expected: "      leftight",
		},
		{
			name:     "less-style cursor home then relative down",
			input:    "\x1b[Hfirst\x1b[1Bsecond\x1b[1Bthird",
			expected: "first\n     second\n           third",
		},
		{
			name:     "mixed absolute and relative cursor movement",
			input:    "\x1b[1;1Hstart\x1b[2Bend\x1b[1;20Hside",
			expected: "start              side\n\n     end",
		},
		{
			name:     "relative movement with default count",
			input:    "\x1b[1;1Hfoo\x1b[Cbar",
			expected: "foo bar",
		},
		// Erase line
		{
			name:     "erase to end of line ESC[K",
			input:    "\x1b[1;1Hhello world\x1b[1;6H\x1b[K",
			expected: "hello",
		},
		{
			name:     "erase to end of line ESC[0K",
			input:    "\x1b[1;1Hhello world\x1b[1;6H\x1b[0K",
			expected: "hello",
		},
		{
			name:     "erase from start of line ESC[1K",
			input:    "\x1b[1;1Hhello world\x1b[1;6H\x1b[1K",
			expected: "      world",
		},
		{
			name:     "erase full line ESC[2K",
			input:    "\x1b[1;1Hhello world\x1b[1;6H\x1b[2K",
			expected: "",
		},
		{
			name:     "erase after positioned content preserves other rows",
			input:    "\x1b[1;1Hkeep\x1b[2;1Hdelete\x1b[2;1H\x1b[2K",
			expected: "keep",
		},
		// DEC Special Graphics charset
		{
			name:     "DEC graphics line drawing q->horizontal line",
			input:    "\x1b[1;1H\x1b(0lqqqqk\x1b(B",
			expected: "┌────┐",
		},
		{
			name:     "DEC graphics box drawing",
			input:    "\x1b[1;1H\x1b(0lqqk\x1b[2;1Hx  x\x1b[3;1Hmqqj\x1b(B",
			expected: "┌──┐\n│  │\n└──┘",
		},
		{
			name:     "DEC graphics switch on and off",
			input:    "\x1b[1;1H\x1b(0q\x1b(Bnormal\x1b(0q\x1b(B",
			expected: "─normal─",
		},
		{
			name:     "DEC graphics vertical line",
			input:    "\x1b[1;1H\x1b(0x\x1b[2;1Hx\x1b[3;1Hx\x1b(B",
			expected: "│\n│\n│",
		},
		// Newline-based grid sizing
		{
			name:     "less-style cursor home with newlines",
			input:    "\x1b[Hline1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "newline count larger than cursor-scanned rows",
			input:    "\x1b[1;1Hheader\nrow2\nrow3\nrow4\nrow5",
			expected: "header\nrow2\nrow3\nrow4\nrow5",
		},
		{
			name:     "no newlines cursor only - existing behavior unchanged",
			input:    "\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3",
			expected: "row1\nrow2\nrow3",
		},
		// Grid clearing on full redraw (cursor home from deep row)
		{
			name: "redraw with shorter lines clears stale content via ESC[H",
			input: "\x1b[1;1Hfirst row long\x1b[2;1Hrow2\x1b[3;1Hrow3\x1b[4;1Hrow4" +
				"\x1b[5;1Hrow5\x1b[6;1Hrow6\x1b[7;1Hrow7\x1b[8;1Hrow8" +
				"\x1b[9;1Hrow9\x1b[10;1Hrow10\x1b[11;1Hrow11" +
				"\x1b[Hshort",
			expected: "short",
		},
		{
			name: "identical redraws preserve content via ESC[1;1H",
			input: "\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3\x1b[4;1Hrow4" +
				"\x1b[5;1Hrow5\x1b[6;1Hrow6\x1b[7;1Hrow7\x1b[8;1Hrow8" +
				"\x1b[9;1Hrow9\x1b[10;1Hrow10\x1b[11;1Hrow11" +
				"\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3\x1b[4;1Hrow4" +
				"\x1b[5;1Hrow5\x1b[6;1Hrow6\x1b[7;1Hrow7\x1b[8;1Hrow8" +
				"\x1b[9;1Hrow9\x1b[10;1Hrow10\x1b[11;1Hrow11",
			expected: "row1\nrow2\nrow3\nrow4\nrow5\nrow6\nrow7\nrow8\nrow9\nrow10\nrow11",
		},
		{
			name:     "cursor home from low row does not clear grid",
			input:    "\x1b[1;1Haaaa\x1b[5;1Hrow5\x1b[Hbb",
			expected: "bbaa\n\n\n\nrow5",
		},
		{
			name: "trailing cursor home for parking does not clear grid",
			input: "\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3\x1b[4;1Hrow4" +
				"\x1b[5;1Hrow5\x1b[6;1Hrow6\x1b[7;1Hrow7\x1b[8;1Hrow8" +
				"\x1b[9;1Hrow9\x1b[10;1Hrow10\x1b[11;1Hrow11" +
				"\x1b[H\x1b[40d\x1b[H",
			expected: "row1\nrow2\nrow3\nrow4\nrow5\nrow6\nrow7\nrow8\nrow9\nrow10\nrow11",
		},
		{
			name: "cursor home followed by ESC sequences only does not clear grid",
			input: "\x1b[1;1Hrow1\x1b[2;1Hrow2\x1b[3;1Hrow3\x1b[4;1Hrow4" +
				"\x1b[5;1Hrow5\x1b[6;1Hrow6\x1b[7;1Hrow7\x1b[8;1Hrow8" +
				"\x1b[9;1Hrow9\x1b[10;1Hrow10\x1b[11;1Hrow11" +
				"\x1b[H\x1b[?25h\x1b[?12l",
			expected: "row1\nrow2\nrow3\nrow4\nrow5\nrow6\nrow7\nrow8\nrow9\nrow10\nrow11",
		},
		// Erase display
		{
			name:     "ESC[2J clears entire grid",
			input:    "\x1b[1;1Hfirst frame\x1b[2;1Hrow2\x1b[H\x1b[2Jsecond",
			expected: "second",
		},
		{
			name: "ncdu-style redraw via ESC[2J after cursor repositioning",
			input: "\x1b[H\x1b[2Jframe1 row1\x1b[2;1Hframe1 row2\x1b[3;1Hframe1 row3" +
				"\x1b[3d\x1b[H\x1b[2Jframe2 short",
			expected: "frame2 short",
		},
		{
			name:     "ESC[J erases from cursor to end",
			input:    "\x1b[1;1Hhello world\x1b[2;1Hrow2\x1b[1;6H\x1b[J",
			expected: "hello",
		},
		{
			name:     "ESC[1J erases from start to cursor",
			input:    "\x1b[1;1Hhello world\x1b[2;1Hrow2\x1b[1;6H\x1b[1J",
			expected: "      world\nrow2",
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
