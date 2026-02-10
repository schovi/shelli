package ansi

import "bytes"

// TruncationStrategy defines which detection methods are enabled for TUI mode.
type TruncationStrategy struct {
	ScreenClear    bool // ESC[2J, ESC[?1049h, ESC c
	SyncMode       bool // ESC[?2026h (sync begin)
	CursorHome     bool // ESC[1;1H with heuristics
	CursorJumpTop  bool // ESC[row;colH jump from high row to row<=2
	MaxSize        int  // Size cap in bytes (0 = disabled)
}

// DefaultTUIStrategy returns the recommended settings for TUI mode.
func DefaultTUIStrategy() TruncationStrategy {
	return TruncationStrategy{
		ScreenClear:   true,
		SyncMode:      true,
		CursorHome:    true,
		CursorJumpTop: true,
		MaxSize:       100 * 1024, // 100KB fallback
	}
}

// FrameDetector detects frame boundaries in PTY output for TUI applications.
// Handles cross-chunk detection via pending buffer.
type FrameDetector struct {
	strategy            TruncationStrategy
	pending             []byte
	heuristicTrail      []byte // trailing bytes from previous chunk for cross-chunk cursor-home lookback
	bufferSize          int    // track accumulated size for MaxSize check
	bytesSinceLastFrame int    // bytes processed since last frame boundary (-1 = never seen)
	seenFrame           bool   // true if we've ever seen a frame boundary
	maxRowSeen          int    // highest row seen in cursor position sequences (for CursorJumpTop)
}

// DetectResult contains the result of processing a chunk.
type DetectResult struct {
	Truncate  bool
	DataAfter []byte
}

// NewFrameDetector creates a new detector with the given strategy.
func NewFrameDetector(strategy TruncationStrategy) *FrameDetector {
	return &FrameDetector{strategy: strategy}
}

// truncation sequences with strategy category
type truncSeq struct {
	seq      []byte
	strategy string
}

var truncationSequences = []truncSeq{
	// Screen clear (priority 1)
	{seq: []byte{0x1B, '[', '2', 'J'}, strategy: "screen_clear"},                     // ESC[2J - clear entire screen
	{seq: []byte{0x1B, '[', '?', '1', '0', '4', '9', 'h'}, strategy: "screen_clear"}, // ESC[?1049h - alt buffer on
	{seq: []byte{0x1B, 'c'}, strategy: "screen_clear"},                               // ESC c - terminal reset

	// Sync mode begin (priority 2) - truncate on frame START, not end
	{seq: []byte{0x1B, '[', '?', '2', '0', '2', '6', 'h'}, strategy: "sync_mode"}, // ESC[?2026h

	// Cursor home (priority 3 - needs heuristic)
	{seq: []byte{0x1B, '[', '1', ';', '1', 'H'}, strategy: "cursor_home"}, // ESC[1;1H
	{seq: []byte{0x1B, '[', 'H'}, strategy: "cursor_home"},                // ESC[H (short form)
}

// cursor home heuristic sequences (must appear within lookback window)
var cursorHomeHeuristics = [][]byte{
	{0x1B, '[', '0', 'm'},        // ESC[0m - reset attributes
	{0x1B, '[', 'm'},             // ESC[m - reset attributes (short)
	{0x1B, '[', '?', '2', '5', 'l'}, // ESC[?25l - hide cursor
}

const (
	maxSequenceLen        = 12
	cursorHomeLookback    = 20
	cursorJumpTopThreshold = 10 // minimum maxRowSeen before a jump to row<=2 counts as truncation
)

// Process analyzes a chunk and returns whether truncation should occur
// and the data to store (everything after the last truncation point).
func (d *FrameDetector) Process(chunk []byte) DetectResult {
	// Combine pending bytes with new chunk
	data := chunk
	if len(d.pending) > 0 {
		data = make([]byte, len(d.pending)+len(chunk))
		copy(data, d.pending)
		copy(data[len(d.pending):], chunk)
		d.pending = nil
	}

	// Find the last truncation sequence position
	lastTruncEnd := -1
	for i := 0; i < len(data); i++ {
		for _, ts := range truncationSequences {
			if !d.strategyEnabled(ts.strategy) {
				continue
			}
			if i+len(ts.seq) <= len(data) && bytes.Equal(data[i:i+len(ts.seq)], ts.seq) {
				if ts.strategy == "cursor_home" {
					if !d.checkCursorHomeHeuristic(data, i) {
						continue
					}
				}
				lastTruncEnd = i + len(ts.seq)
				d.maxRowSeen = 0
			}
		}

		// Cursor jump-to-top detection
		if d.strategy.CursorJumpTop && data[i] == 0x1B {
			if row, end, ok := parseCursorRow(data, i); ok {
				if row <= 2 && d.maxRowSeen >= cursorJumpTopThreshold {
					lastTruncEnd = end
					d.maxRowSeen = 0
				} else if row > d.maxRowSeen {
					d.maxRowSeen = row
				}
			}
		}
	}

	// Buffer trailing bytes that could be start of escape sequence
	pendingStart := len(data)
	if len(data) > 0 {
		for i := max(0, len(data)-maxSequenceLen); i < len(data); i++ {
			if data[i] == 0x1B {
				remaining := data[i:]
				for _, ts := range truncationSequences {
					if !d.strategyEnabled(ts.strategy) {
						continue
					}
					if isPrefixOf(remaining, ts.seq) && len(remaining) < len(ts.seq) {
						pendingStart = i
						break
					}
				}
				if pendingStart == len(data) && d.strategy.CursorJumpTop {
					if isPartialCursorPosition(remaining) {
						pendingStart = i
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

	if d.strategy.CursorHome {
		if len(data) >= cursorHomeLookback {
			d.heuristicTrail = make([]byte, cursorHomeLookback)
			copy(d.heuristicTrail, data[len(data)-cursorHomeLookback:])
		} else if len(data) > 0 {
			d.heuristicTrail = make([]byte, len(data))
			copy(d.heuristicTrail, data)
		}
	}

	// Check for recent frame BEFORE updating (to catch size cap before recency expires)
	hadRecentFrame := d.seenFrame && d.bytesSinceLastFrame < d.frameRecencyWindow()

	// Update buffer size tracking and frame recency
	if lastTruncEnd == -1 {
		d.bufferSize += len(data)
		d.bytesSinceLastFrame += len(data)
	} else {
		d.seenFrame = true
		d.bytesSinceLastFrame = len(data) - lastTruncEnd
		d.bufferSize = d.bytesSinceLastFrame
	}

	// Check size cap (priority 4) - only if frame boundary was seen recently
	// This prevents breaking hybrid TUI apps (btm, etc.) that send frames once at startup
	// then switch to incremental updates
	if d.strategy.MaxSize > 0 && hadRecentFrame && d.bufferSize > d.strategy.MaxSize {
		d.bufferSize = len(data)
		d.bytesSinceLastFrame = len(data)
		return DetectResult{
			Truncate:  true,
			DataAfter: data,
		}
	}

	if lastTruncEnd == -1 {
		return DetectResult{
			Truncate:  false,
			DataAfter: data,
		}
	}

	// Return only data after the last truncation point
	afterTrunc := data[lastTruncEnd:]
	return DetectResult{
		Truncate:  true,
		DataAfter: afterTrunc,
	}
}

// strategyEnabled checks if the given strategy type is enabled.
func (d *FrameDetector) strategyEnabled(strategy string) bool {
	switch strategy {
	case "screen_clear":
		return d.strategy.ScreenClear
	case "sync_mode":
		return d.strategy.SyncMode
	case "cursor_home":
		return d.strategy.CursorHome
	case "cursor_jump_top":
		return d.strategy.CursorJumpTop
	default:
		return false
	}
}

// checkCursorHomeHeuristic returns true if cursor home should trigger truncation.
// Only triggers if preceded by reset or hide cursor within lookback window.
// Uses heuristicTrail from the previous chunk when the lookback window extends
// beyond the start of the current data.
func (d *FrameDetector) checkCursorHomeHeuristic(data []byte, pos int) bool {
	var window []byte
	switch {
	case pos >= cursorHomeLookback:
		window = data[pos-cursorHomeLookback : pos]
	case len(d.heuristicTrail) > 0:
		// Extend lookback into previous chunk's trailing bytes
		need := cursorHomeLookback - pos
		trail := d.heuristicTrail
		if need > len(trail) {
			need = len(trail)
		}
		window = make([]byte, need+pos)
		copy(window, trail[len(trail)-need:])
		copy(window[need:], data[:pos])
	default:
		window = data[:pos]
	}

	for _, heuristic := range cursorHomeHeuristics {
		for i := 0; i <= len(window)-len(heuristic); i++ {
			if bytes.Equal(window[i:i+len(heuristic)], heuristic) {
				return true
			}
		}
	}
	return false
}

// Reset clears all detector state (pending buffer, heuristics, counters).
// Use before triggering a fresh TUI redraw (e.g., snapshot resize cycle).
func (d *FrameDetector) Reset() {
	d.pending = nil
	d.heuristicTrail = nil
	d.bufferSize = 0
	d.bytesSinceLastFrame = 0
	d.seenFrame = false
	d.maxRowSeen = 0
}

// ResetBufferSize resets the buffer size tracker (call after external truncation).
func (d *FrameDetector) ResetBufferSize() {
	d.bufferSize = 0
}

// BufferSize returns the current tracked buffer size.
func (d *FrameDetector) BufferSize() int {
	return d.bufferSize
}

// HasRecentFrame returns true if a frame boundary was detected recently
// (within the last frameRecencyWindow bytes).
func (d *FrameDetector) HasRecentFrame() bool {
	return d.seenFrame && d.bytesSinceLastFrame < d.frameRecencyWindow()
}

// frameRecencyWindow returns MaxSize * 2, ensuring the recency window is always
// large enough for size-cap to fire before recency expires.
func (d *FrameDetector) frameRecencyWindow() int {
	return d.strategy.MaxSize * 2
}

// BytesSinceLastFrame returns bytes processed since the last frame boundary.
func (d *FrameDetector) BytesSinceLastFrame() int {
	return d.bytesSinceLastFrame
}

// Flush returns any pending bytes (call when session ends).
func (d *FrameDetector) Flush() []byte {
	pending := d.pending
	d.pending = nil
	return pending
}

// isPartialCursorPosition returns true if data looks like the start of an
// incomplete ESC[row;colH or ESC[row;colF sequence.
func isPartialCursorPosition(data []byte) bool {
	if len(data) < 1 || data[0] != 0x1B {
		return false
	}
	if len(data) < 2 {
		return true // just ESC, could be anything
	}
	if data[1] != '[' {
		return false
	}
	// ESC[ followed by digits, optional semicolons, and more digits (but no terminator yet)
	for j := 2; j < len(data); j++ {
		if data[j] >= '0' && data[j] <= '9' {
			continue
		}
		if data[j] == ';' {
			continue
		}
		// Hit a non-digit, non-semicolon: this is a complete (or invalid) sequence
		return false
	}
	// Ended without terminator: partial
	return len(data) > 2
}

// parseCursorRow parses ESC[row;colH or ESC[row;colF at position i in data.
// Returns the row number, the byte index after the sequence, and whether parsing succeeded.
// Expects data[i] == 0x1B and data[i+1] == '['.
func parseCursorRow(data []byte, i int) (row int, endIndex int, ok bool) {
	if i+2 >= len(data) || data[i] != 0x1B || data[i+1] != '[' {
		return 0, 0, false
	}

	j := i + 2
	row = 0
	hasRow := false

	// Parse row number
	for j < len(data) && data[j] >= '0' && data[j] <= '9' {
		row = row*10 + int(data[j]-'0')
		hasRow = true
		j++
	}

	if j >= len(data) {
		return 0, 0, false
	}

	// After row, expect ';' then col then 'H'/'F', OR just 'H'/'F' (row-only form)
	if data[j] == ';' {
		j++ // skip ';'
		// Parse col number (we don't need the value)
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j >= len(data) {
			return 0, 0, false
		}
	}

	if data[j] == 'H' || data[j] == 'F' {
		if !hasRow {
			row = 1 // ESC[H defaults to row 1
		}
		return row, j + 1, true
	}

	return 0, 0, false
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
