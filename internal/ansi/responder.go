package ansi

import (
	"fmt"
	"os"
	"sync"
)

// TerminalResponder intercepts terminal capability queries in PTY output
// and writes appropriate responses to the PTY master fd (appearing as input
// to the subprocess). This unblocks apps like yazi that wait for DA1/DA2/etc.
type TerminalResponder struct {
	mu   sync.Mutex
	ptmx *os.File
	cols int
	rows int
}

// NewTerminalResponder creates a responder that writes responses to ptmx.
func NewTerminalResponder(ptmx *os.File, cols, rows int) *TerminalResponder {
	return &TerminalResponder{ptmx: ptmx, cols: cols, rows: rows}
}

// SetSize updates the terminal dimensions used for DSR responses.
func (r *TerminalResponder) SetSize(cols, rows int) {
	r.mu.Lock()
	r.cols = cols
	r.rows = rows
	r.mu.Unlock()
}

// Process scans data for terminal queries, sends responses to the PTY,
// and returns data with query sequences stripped out.
func (r *TerminalResponder) Process(data []byte) []byte {
	// Fast path: no ESC in data
	hasESC := false
	for _, b := range data {
		if b == 0x1B {
			hasESC = true
			break
		}
	}
	if !hasESC {
		return data
	}

	result := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] != 0x1B {
			result = append(result, data[i])
			i++
			continue
		}

		// Try to match a query sequence starting at i
		consumed, response := r.matchQuery(data, i)
		if consumed > 0 {
			if response != "" {
				r.respond(response)
			}
			i += consumed
			continue
		}

		// Not a recognized query, pass through
		result = append(result, data[i])
		i++
	}

	return result
}

// matchQuery attempts to match a terminal query at data[pos].
// Returns (bytes consumed, response to send). Zero consumed means no match.
func (r *TerminalResponder) matchQuery(data []byte, pos int) (int, string) {
	remaining := len(data) - pos
	if remaining < 2 {
		return 0, ""
	}

	// All queries start with ESC[
	if data[pos+1] != '[' {
		return 0, ""
	}

	if remaining < 3 {
		return 0, ""
	}

	// DA1: ESC[c or ESC[0c
	if data[pos+2] == 'c' {
		// VT220 with various capabilities
		return 3, "\x1b[?62;1;2;6;7;8;9;15;22c"
	}
	if remaining >= 4 && data[pos+2] == '0' && data[pos+3] == 'c' {
		return 4, "\x1b[?62;1;2;6;7;8;9;15;22c"
	}

	// DA2: ESC[>c or ESC[>0c
	if data[pos+2] == '>' {
		if remaining >= 4 && data[pos+3] == 'c' {
			return 4, "\x1b[>1;1;0c"
		}
		if remaining >= 5 && data[pos+3] == '0' && data[pos+4] == 'c' {
			return 5, "\x1b[>1;1;0c"
		}
	}

	// DSR (cursor position): ESC[6n
	if remaining >= 4 && data[pos+2] == '6' && data[pos+3] == 'n' {
		return 4, "\x1b[1;1R"
	}

	// Kitty keyboard query: ESC[?u
	if remaining >= 4 && data[pos+2] == '?' && data[pos+3] == 'u' {
		return 4, "\x1b[?0u"
	}

	// DECRPM mode query: ESC[?{digits}$p
	if data[pos+2] == '?' {
		j := pos + 3
		modeStart := j
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j > modeStart && j+1 < len(data) && data[j] == '$' && data[j+1] == 'p' {
			modeStr := string(data[modeStart:j])
			return j + 2 - pos, fmt.Sprintf("\x1b[?%s;0$y", modeStr)
		}
	}

	return 0, ""
}

func (r *TerminalResponder) respond(response string) {
	r.mu.Lock()
	ptmx := r.ptmx
	r.mu.Unlock()

	if ptmx != nil {
		ptmx.Write([]byte(response))
	}
}
