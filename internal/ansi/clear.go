package ansi

import "bytes"

// TruncationStrategy defines which detection methods are enabled for TUI mode.
type TruncationStrategy struct {
	ScreenClear bool // ESC[2J, ESC[?1049h, ESC c
	SyncMode    bool // ESC[?2026h (sync begin)
	CursorHome  bool // ESC[1;1H with heuristics
	MaxSize     int  // Size cap in bytes (0 = disabled)
}

// DefaultTUIStrategy returns the recommended settings for TUI mode.
func DefaultTUIStrategy() TruncationStrategy {
	return TruncationStrategy{
		ScreenClear: true,
		SyncMode:    true,
		CursorHome:  true,
		MaxSize:     100 * 1024, // 100KB fallback
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
	maxSequenceLen     = 8
	cursorHomeLookback = 20
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
