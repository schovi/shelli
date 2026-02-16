package ansi

import (
	"bytes"
	"sync"
)

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

// pendingTruncation tracks a deferred truncation that needs look-ahead
// resolution in the next chunk (used by CursorJumpTop and CursorHome).
type pendingTruncation struct {
	active   bool
	truncEnd int
}

// FrameDetector detects frame boundaries in PTY output for TUI applications.
// Handles cross-chunk detection via pending buffer. Thread-safe.
type FrameDetector struct {
	mu                  sync.Mutex
	strategy            TruncationStrategy
	pending             []byte
	heuristicTrail      []byte // trailing bytes from previous chunk for cross-chunk cursor-home lookback
	bufferSize          int    // track accumulated size for MaxSize check
	bytesSinceLastFrame int    // bytes processed since last frame boundary (-1 = never seen)
	seenFrame           bool   // true if we've ever seen a frame boundary
	maxRowSeen        int              // highest row seen in cursor position sequences (for CursorJumpTop)
	snapshotMode      bool             // when true, all truncation is suppressed
	pendingJump       pendingTruncation // deferred CursorJumpTop needing look-ahead in next chunk
	pendingHome       pendingTruncation // deferred cursor_home needing look-ahead in next chunk
	lastCursorHomePos int              // byte position of last cursor_home truncation within current Process() call (-1 = none)
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

// strategyGroup groups truncation sequences by the strategy flag that enables them.
type strategyGroup struct {
	enabled    func(s *TruncationStrategy, snapshotMode bool) bool
	seqs       [][]byte
	cursorHome bool // requires heuristic + cooldown + look-ahead
}

var truncationGroups = []strategyGroup{
	{
		enabled: func(s *TruncationStrategy, snap bool) bool { return !snap && s.ScreenClear },
		seqs: [][]byte{
			{0x1B, '[', '2', 'J'},                     // ESC[2J - clear entire screen
			{0x1B, '[', '?', '1', '0', '4', '9', 'h'}, // ESC[?1049h - alt buffer on
			{0x1B, 'c'},                                // ESC c - terminal reset
		},
	},
	{
		enabled: func(s *TruncationStrategy, snap bool) bool { return !snap && s.SyncMode },
		seqs: [][]byte{
			{0x1B, '[', '?', '2', '0', '2', '6', 'h'}, // ESC[?2026h - sync begin
		},
	},
	{
		enabled:    func(s *TruncationStrategy, snap bool) bool { return !snap && s.CursorHome },
		cursorHome: true,
		seqs: [][]byte{
			{0x1B, '[', '1', ';', '1', 'H'}, // ESC[1;1H
			{0x1B, '[', 'H'},                // ESC[H (short form)
		},
	},
}

// cursor home heuristic sequences (must appear within lookback window)
var cursorHomeHeuristics = [][]byte{
	{0x1B, '[', '0', 'm'},        // ESC[0m - reset attributes
	{0x1B, '[', 'm'},             // ESC[m - reset attributes (short)
	{0x1B, '[', '?', '2', '5', 'l'}, // ESC[?25l - hide cursor
}

const (
	maxSequenceLen         = 12
	cursorHomeLookback     = 20
	cursorJumpTopThreshold = 10   // minimum maxRowSeen before a jump to row<=2 counts as truncation
	jumpLookAheadLen       = 50   // bytes to scan forward after a cursor jump to check for content
	cursorHomeCooldownBytes = 4096 // bytes to suppress cursor_home after it fires
)

// Process analyzes a chunk and returns whether truncation should occur
// and the data to store (everything after the last truncation point).
func (d *FrameDetector) Process(chunk []byte) DetectResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Combine pending bytes with new chunk
	data := chunk
	if len(d.pending) > 0 {
		data = make([]byte, len(d.pending)+len(chunk))
		copy(data, d.pending)
		copy(data[len(d.pending):], chunk)
		d.pending = nil
	}

	// Track cursor_home cooldown within this Process() call
	d.lastCursorHomePos = -1

	lastTruncEnd := d.resolvePending(data)

	// Find the last truncation sequence position
	for i := 0; i < len(data); i++ {
		for _, g := range truncationGroups {
			if !g.enabled(&d.strategy, d.snapshotMode) {
				continue
			}
			for _, seq := range g.seqs {
				if i+len(seq) > len(data) || !bytes.Equal(data[i:i+len(seq)], seq) {
					continue
				}
				if g.cursorHome {
					if d.lastCursorHomePos >= 0 && i-d.lastCursorHomePos < cursorHomeCooldownBytes {
						continue
					}
					if !d.checkCursorHomeHeuristic(data, i) {
						continue
					}
					end := i + len(seq)
					switch {
					case d.hasContentAfterCursor(data, end):
						d.lastCursorHomePos = i
					case end >= len(data):
						d.pendingHome = pendingTruncation{active: true, truncEnd: end}
						continue
					default:
						continue
					}
				}
				lastTruncEnd = i + len(seq)
				d.maxRowSeen = 0
			}
		}

		// Cursor jump-to-top detection with look-ahead
		if d.strategy.CursorJumpTop && !d.snapshotMode && data[i] == 0x1B {
			if row, end, ok := parseCursorRow(data, i); ok {
				if row <= 2 && d.maxRowSeen >= cursorJumpTopThreshold {
					if d.hasContentAfterCursor(data, end) {
						lastTruncEnd = end
						d.maxRowSeen = 0
					} else if end >= len(data)-maxSequenceLen {
						d.pendingJump = pendingTruncation{active: true, truncEnd: end}
					}
				} else if row > d.maxRowSeen {
					d.maxRowSeen = row
				}
			}
		}
	}

	data = d.bufferTrailingBytes(data)

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

	if result := d.checkSizeCap(data, hadRecentFrame); result != nil {
		return *result
	}

	if lastTruncEnd == -1 {
		return DetectResult{Truncate: false, DataAfter: data}
	}

	return DetectResult{Truncate: true, DataAfter: data[lastTruncEnd:]}
}

// resolvePending checks for deferred truncations from the previous chunk.
// If the deferred jump/home had content following in this new chunk,
// the truncation is confirmed (returns 0). Otherwise returns -1 (no truncation).
func (d *FrameDetector) resolvePending(data []byte) int {
	lastTruncEnd := -1
	if d.pendingJump.active {
		d.pendingJump = pendingTruncation{}
		if d.hasContentAfterCursor(data, 0) {
			lastTruncEnd = 0
			d.maxRowSeen = 0
		}
	}
	if d.pendingHome.active {
		d.pendingHome = pendingTruncation{}
		if d.hasContentAfterCursor(data, 0) {
			lastTruncEnd = 0
		}
	}
	return lastTruncEnd
}

// bufferTrailingBytes moves trailing bytes that could be the start of an
// incomplete escape sequence into d.pending for the next Process() call.
// Returns data with those trailing bytes removed.
func (d *FrameDetector) bufferTrailingBytes(data []byte) []byte {
	pendingStart := len(data)
	if len(data) > 0 {
		for i := max(0, len(data)-maxSequenceLen); i < len(data); i++ {
			if data[i] == 0x1B {
				remaining := data[i:]
				for _, g := range truncationGroups {
					if !g.enabled(&d.strategy, d.snapshotMode) {
						continue
					}
					for _, seq := range g.seqs {
						if isPrefixOf(remaining, seq) && len(remaining) < len(seq) {
							pendingStart = i
							break
						}
					}
					if pendingStart != len(data) {
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
	return data
}

// checkSizeCap applies the MaxSize truncation strategy.
// Only fires when a frame boundary was seen recently (prevents breaking hybrid TUI apps
// that send frames once at startup then switch to incremental updates).
func (d *FrameDetector) checkSizeCap(data []byte, hadRecentFrame bool) *DetectResult {
	if d.strategy.MaxSize > 0 && !d.snapshotMode && hadRecentFrame && d.bufferSize > d.strategy.MaxSize {
		d.bufferSize = len(data)
		d.bytesSinceLastFrame = len(data)
		return &DetectResult{Truncate: true, DataAfter: data}
	}
	return nil
}

// SetSnapshotMode enables or disables snapshot mode.
// When enabled, ALL truncation strategies are suppressed (screen_clear,
// sync_mode, cursor_home, CursorJumpTop, MaxSize). The settle timer
// determines frame completion during snapshot, not frame detection.
func (d *FrameDetector) SetSnapshotMode(enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.snapshotMode = enabled
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

// hasContentAfterCursor scans forward from pos in data (up to jumpLookAheadLen bytes)
// to determine if printable content follows. Skips escape sequences (colors, modes,
// cursor control). Returns true if printable content is found, false if only
// cursor control / DEC private mode sequences follow.
func (d *FrameDetector) hasContentAfterCursor(data []byte, pos int) bool {
	limit := pos + jumpLookAheadLen
	if limit > len(data) {
		limit = len(data)
	}
	j := pos
	for j < limit {
		if data[j] == 0x1B {
			// Skip escape sequences
			if j+1 < limit && data[j+1] == '[' {
				// CSI sequence: ESC[ ... terminator
				k := j + 2
				for k < limit && !isCSITerminator(data[k]) {
					k++
				}
				if k < limit {
					k++ // skip terminator
				}
				j = k
				continue
			}
			// Other ESC sequences (ESC + one char, or ESC( / ESC))
			if j+2 < limit && (data[j+1] == '(' || data[j+1] == ')' || data[j+1] == '#') {
				j += 3
				continue
			}
			j += 2
			continue
		}
		// Printable character found (not ESC, not control)
		if data[j] >= 0x20 && data[j] != 0x7F {
			return true
		}
		j++
	}
	return false
}

// Reset clears all detector state (pending buffer, heuristics, counters).
// Use before triggering a fresh TUI redraw (e.g., snapshot resize cycle).
func (d *FrameDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = nil
	d.heuristicTrail = nil
	d.bufferSize = 0
	d.bytesSinceLastFrame = 0
	d.seenFrame = false
	d.maxRowSeen = 0
	d.pendingJump = pendingTruncation{}
	d.pendingHome = pendingTruncation{}
	d.lastCursorHomePos = -1
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
	d.mu.Lock()
	defer d.mu.Unlock()
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

	// Parse row number (cap at 5 digits to prevent overflow)
	digits := 0
	for j < len(data) && data[j] >= '0' && data[j] <= '9' {
		digits++
		if digits > 5 {
			return 0, 0, false
		}
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
		// Parse col number (we don't need the value, cap at 5 digits)
		colDigits := 0
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			colDigits++
			if colDigits > 5 {
				return 0, 0, false
			}
			j++
		}
		if j >= len(data) {
			return 0, 0, false
		}
	}

	if data[j] == 'H' || data[j] == 'F' || data[j] == 'f' {
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
