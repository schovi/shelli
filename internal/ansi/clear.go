package ansi

// ScreenClearDetector detects screen clear sequences in PTY output.
// Handles cross-chunk detection via pending buffer.
type ScreenClearDetector struct {
	pending []byte
}

// ClearResult contains the result of processing a chunk.
type ClearResult struct {
	ClearFound bool
	DataAfter  []byte
}

// NewScreenClearDetector creates a new detector.
func NewScreenClearDetector() *ScreenClearDetector {
	return &ScreenClearDetector{}
}

// Screen clear sequences to detect:
// - ESC[2J (0x1B 0x5B 0x32 0x4A) - full screen clear
// - ESC[?1049h - alternate buffer on (switch to alternate screen)
// - ESC c (0x1B 0x63) - terminal reset (RIS)
var clearSequences = [][]byte{
	{0x1B, '[', '2', 'J'},     // ESC[2J - clear entire screen
	{0x1B, '[', '?', '1', '0', '4', '9', 'h'}, // ESC[?1049h - alt buffer on
	{0x1B, 'c'},               // ESC c - terminal reset
}

const maxSequenceLen = 8

// Process analyzes a chunk and returns whether a clear was found
// and the data to store (everything after the last clear).
func (d *ScreenClearDetector) Process(chunk []byte) ClearResult {
	// Combine pending bytes with new chunk
	data := chunk
	if len(d.pending) > 0 {
		data = make([]byte, len(d.pending)+len(chunk))
		copy(data, d.pending)
		copy(data[len(d.pending):], chunk)
		d.pending = nil
	}

	// Find the last clear sequence position
	lastClearEnd := -1
	for i := 0; i < len(data); i++ {
		for _, seq := range clearSequences {
			if i+len(seq) <= len(data) && matchBytes(data[i:i+len(seq)], seq) {
				lastClearEnd = i + len(seq)
			}
		}
	}

	// Buffer trailing bytes that could be start of escape sequence
	// Only if data ends with ESC or partial sequence
	pendingStart := len(data)
	if len(data) > 0 {
		for i := max(0, len(data)-maxSequenceLen); i < len(data); i++ {
			if data[i] == 0x1B {
				// Check if this could be start of any clear sequence
				remaining := data[i:]
				for _, seq := range clearSequences {
					if isPrefixOf(remaining, seq) && len(remaining) < len(seq) {
						pendingStart = i
						break
					}
				}
				if pendingStart != len(data) {
					break
				}
			}
		}
	}

	if pendingStart < len(data) {
		d.pending = make([]byte, len(data)-pendingStart)
		copy(d.pending, data[pendingStart:])
		data = data[:pendingStart]
	}

	if lastClearEnd == -1 {
		return ClearResult{
			ClearFound: false,
			DataAfter:  data,
		}
	}

	// Return only data after the last clear
	afterClear := data[lastClearEnd:]
	return ClearResult{
		ClearFound: true,
		DataAfter:  afterClear,
	}
}

// Flush returns any pending bytes (call when session ends).
func (d *ScreenClearDetector) Flush() []byte {
	pending := d.pending
	d.pending = nil
	return pending
}

func matchBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isPrefixOf(data, seq []byte) bool {
	if len(data) > len(seq) {
		return false
	}
	for i := range data {
		if data[i] != seq[i] {
			return false
		}
	}
	return true
}
