package ansi

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestScreenClearDetector(t *testing.T) {
	tests := []struct {
		name       string
		chunks     [][]byte
		wantClears []bool
		wantData   [][]byte
	}{
		{
			name:       "no clear sequence",
			chunks:     [][]byte{[]byte("hello world")},
			wantClears: []bool{false},
			wantData:   [][]byte{[]byte("hello world")},
		},
		{
			name:       "ESC[2J clears screen",
			chunks:     [][]byte{[]byte("old\x1b[2Jnew")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte("new")},
		},
		{
			name:       "ESC[?1049h alternate buffer",
			chunks:     [][]byte{[]byte("old\x1b[?1049hnew")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte("new")},
		},
		{
			name:       "ESC c terminal reset",
			chunks:     [][]byte{[]byte("old\x1bcnew")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte("new")},
		},
		{
			name:       "multiple clears - last one wins",
			chunks:     [][]byte{[]byte("first\x1b[2Jsecond\x1b[2Jthird")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte("third")},
		},
		{
			name:       "clear at end of chunk",
			chunks:     [][]byte{[]byte("old content\x1b[2J")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte{}},
		},
		{
			name:       "clear at start of chunk",
			chunks:     [][]byte{[]byte("\x1b[2Jnew content")},
			wantClears: []bool{true},
			wantData:   [][]byte{[]byte("new content")},
		},
		{
			name: "cross-chunk ESC[2J - ESC at end",
			chunks: [][]byte{
				[]byte("old\x1b"),
				[]byte("[2Jnew"),
			},
			wantClears: []bool{false, true},
			wantData:   [][]byte{[]byte("old"), []byte("new")},
		},
		{
			name: "cross-chunk ESC[2J - ESC[ at end",
			chunks: [][]byte{
				[]byte("old\x1b["),
				[]byte("2Jnew"),
			},
			wantClears: []bool{false, true},
			wantData:   [][]byte{[]byte("old"), []byte("new")},
		},
		{
			name: "cross-chunk ESC[2J - ESC[2 at end",
			chunks: [][]byte{
				[]byte("old\x1b[2"),
				[]byte("Jnew"),
			},
			wantClears: []bool{false, true},
			wantData:   [][]byte{[]byte("old"), []byte("new")},
		},
		{
			name: "cross-chunk alternate buffer",
			chunks: [][]byte{
				[]byte("old\x1b[?104"),
				[]byte("9hnew"),
			},
			wantClears: []bool{false, true},
			wantData:   [][]byte{[]byte("old"), []byte("new")},
		},
		{
			name: "no clear across multiple chunks",
			chunks: [][]byte{
				[]byte("hello "),
				[]byte("world "),
				[]byte("test"),
			},
			wantClears: []bool{false, false, false},
			wantData:   [][]byte{[]byte("hello "), []byte("world "), []byte("test")},
		},
		{
			name:       "empty chunk",
			chunks:     [][]byte{[]byte{}},
			wantClears: []bool{false},
			wantData:   [][]byte{[]byte{}},
		},
		{
			name: "partial ESC not a clear",
			chunks: [][]byte{
				[]byte("hello\x1b[31mred"),
			},
			wantClears: []bool{false},
			wantData:   [][]byte{[]byte("hello\x1b[31mred")},
		},
		{
			name: "ESC alone then regular text",
			chunks: [][]byte{
				[]byte("data\x1b"),
				[]byte("regular text"),
			},
			wantClears: []bool{false, false},
			wantData:   [][]byte{[]byte("data"), []byte("\x1bregular text")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewFrameDetector(TruncationStrategy{ScreenClear: true})

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.Truncate != tt.wantClears[i] {
					t.Errorf("chunk %d: Truncate = %v, want %v", i, result.Truncate, tt.wantClears[i])
				}

				if !bytes.Equal(result.DataAfter, tt.wantData[i]) {
					t.Errorf("chunk %d: DataAfter = %q, want %q", i, result.DataAfter, tt.wantData[i])
				}
			}
		})
	}
}

func TestScreenClearDetector_Flush(t *testing.T) {
	d := NewFrameDetector(TruncationStrategy{ScreenClear: true})

	// Send chunk ending with partial escape sequence
	result := d.Process([]byte("data\x1b"))
	if result.Truncate {
		t.Error("expected no clear")
	}
	if !bytes.Equal(result.DataAfter, []byte("data")) {
		t.Errorf("DataAfter = %q, want %q", result.DataAfter, "data")
	}

	// Flush should return pending bytes
	pending := d.Flush()
	if !bytes.Equal(pending, []byte("\x1b")) {
		t.Errorf("Flush() = %q, want %q", pending, "\x1b")
	}

	// Second flush should return nil
	pending = d.Flush()
	if pending != nil {
		t.Errorf("second Flush() = %q, want nil", pending)
	}
}

func TestScreenClearDetector_RealWorldVim(t *testing.T) {
	// Simulate vim startup which uses alternate buffer
	d := NewFrameDetector(TruncationStrategy{ScreenClear: true})

	// vim typically sends: ESC[?1049h to enter alternate buffer
	chunks := [][]byte{
		[]byte("normal shell output\n"),
		[]byte("$ vim file.txt\n"),
		[]byte("\x1b[?1049h"), // enter alternate buffer
		[]byte("\x1b[2J"),     // clear screen
		[]byte("file contents here\n"),
	}

	var totalClears int
	var lastData []byte

	for _, chunk := range chunks {
		result := d.Process(chunk)
		if result.Truncate {
			totalClears++
		}
		lastData = result.DataAfter
	}

	// Should have detected clears
	if totalClears < 2 {
		t.Errorf("expected at least 2 clears, got %d", totalClears)
	}

	// Last data should be the file contents
	if !bytes.Equal(lastData, []byte("file contents here\n")) {
		t.Errorf("last data = %q, want %q", lastData, "file contents here\n")
	}
}

func TestFrameDetector_SyncMode(t *testing.T) {
	tests := []struct {
		name       string
		chunks     [][]byte
		wantTrunc  []bool
		wantData   [][]byte
	}{
		{
			name:      "sync mode begin truncates (new frame starts)",
			chunks:    [][]byte{[]byte("oldframe\x1b[?2026hnewframe")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("newframe")},
		},
		{
			name:      "sync mode end does not truncate",
			chunks:    [][]byte{[]byte("frame\x1b[?2026lafter")},
			wantTrunc: []bool{false},
			wantData:  [][]byte{[]byte("frame\x1b[?2026lafter")},
		},
		{
			name:      "multiple sync frames - last begin wins",
			chunks:    [][]byte{[]byte("\x1b[?2026hf1\x1b[?2026l\x1b[?2026hf2\x1b[?2026l")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("f2\x1b[?2026l")},
		},
		{
			name: "cross-chunk sync mode begin",
			chunks: [][]byte{
				[]byte("old\x1b[?202"),
				[]byte("6hnew"),
			},
			wantTrunc: []bool{false, true},
			wantData:  [][]byte{[]byte("old"), []byte("new")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewFrameDetector(TruncationStrategy{
				SyncMode: true,
			})

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.Truncate != tt.wantTrunc[i] {
					t.Errorf("chunk %d: Truncate = %v, want %v", i, result.Truncate, tt.wantTrunc[i])
				}

				if !bytes.Equal(result.DataAfter, tt.wantData[i]) {
					t.Errorf("chunk %d: DataAfter = %q, want %q", i, result.DataAfter, tt.wantData[i])
				}
			}
		})
	}
}

func TestFrameDetector_CursorHome(t *testing.T) {
	tests := []struct {
		name       string
		chunks     [][]byte
		wantTrunc  []bool
		wantData   [][]byte
	}{
		{
			name:      "cursor home with reset truncates",
			chunks:    [][]byte{[]byte("old\x1b[0m\x1b[1;1Hnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "cursor home with short reset truncates",
			chunks:    [][]byte{[]byte("old\x1b[m\x1b[1;1Hnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "cursor home with hide cursor truncates",
			chunks:    [][]byte{[]byte("old\x1b[?25l\x1b[1;1Hnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "cursor home without heuristic does not truncate",
			chunks:    [][]byte{[]byte("old\x1b[1;1Hnew")},
			wantTrunc: []bool{false},
			wantData:  [][]byte{[]byte("old\x1b[1;1Hnew")},
		},
		{
			name:      "short cursor home with reset truncates",
			chunks:    [][]byte{[]byte("old\x1b[0m\x1b[Hnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "short cursor home without heuristic does not truncate",
			chunks:    [][]byte{[]byte("old\x1b[Hnew")},
			wantTrunc: []bool{false},
			wantData:  [][]byte{[]byte("old\x1b[Hnew")},
		},
		{
			name:      "heuristic too far away does not truncate",
			chunks:    [][]byte{[]byte("old\x1b[0m" + string(make([]byte, 25)) + "\x1b[1;1Hnew")},
			wantTrunc: []bool{false},
			wantData:  [][]byte{[]byte("old\x1b[0m" + string(make([]byte, 25)) + "\x1b[1;1Hnew")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewFrameDetector(TruncationStrategy{
				CursorHome: true,
			})

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.Truncate != tt.wantTrunc[i] {
					t.Errorf("chunk %d: Truncate = %v, want %v", i, result.Truncate, tt.wantTrunc[i])
				}

				if !bytes.Equal(result.DataAfter, tt.wantData[i]) {
					t.Errorf("chunk %d: DataAfter = %q, want %q", i, result.DataAfter, tt.wantData[i])
				}
			}
		})
	}
}

func TestFrameDetector_SizeCap(t *testing.T) {
	tests := []struct {
		name          string
		maxSize       int
		strategy      TruncationStrategy
		chunks        [][]byte
		wantTrunc     []bool
		wantMaxLen    []int // max length of DataAfter
	}{
		{
			name:       "under cap no truncation",
			maxSize:    1000,
			strategy:   TruncationStrategy{ScreenClear: true, MaxSize: 1000},
			chunks:     [][]byte{[]byte("\x1b[2Jhello world")},
			wantTrunc:  []bool{true},
			wantMaxLen: []int{100},
		},
		{
			name:     "exceeds cap truncates after frame boundary",
			maxSize:  50,
			strategy: TruncationStrategy{ScreenClear: true, MaxSize: 50},
			chunks:   [][]byte{[]byte("\x1b[2J"), make([]byte, 100)},
			wantTrunc:  []bool{true, true},
			wantMaxLen: []int{0, 100},
		},
		{
			name:     "accumulated size triggers cap after frame",
			maxSize:  50,
			strategy: TruncationStrategy{ScreenClear: true, MaxSize: 50},
			chunks:   [][]byte{[]byte("\x1b[2J"), make([]byte, 30), make([]byte, 30)},
			wantTrunc:  []bool{true, false, true},
			wantMaxLen: []int{0, 30, 30},
		},
		{
			name:       "disabled cap (0) never truncates on size",
			maxSize:    0,
			strategy:   TruncationStrategy{MaxSize: 0},
			chunks:     [][]byte{make([]byte, 1000)},
			wantTrunc:  []bool{false},
			wantMaxLen: []int{1000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewFrameDetector(tt.strategy)

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.Truncate != tt.wantTrunc[i] {
					t.Errorf("chunk %d: Truncate = %v, want %v", i, result.Truncate, tt.wantTrunc[i])
				}

				if len(result.DataAfter) > tt.wantMaxLen[i] {
					t.Errorf("chunk %d: DataAfter len = %d, want <= %d", i, len(result.DataAfter), tt.wantMaxLen[i])
				}
			}
		})
	}
}

func TestFrameDetector_AdaptiveTUI(t *testing.T) {
	t.Run("incremental app without frame boundaries - no size cap truncation", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// Simulate incremental TUI: positional updates only, no frame sequences
		chunks := [][]byte{
			[]byte("initial full screen draw\n"),
			[]byte("\x1b[5;10Hupdate cell"),  // positional update (row 5, col 10)
			[]byte("\x1b[10;20Hanother cell"), // another positional update
		}

		// Send lots of data to exceed size cap
		bigChunk := make([]byte, 150*1024) // 150KB, exceeds 100KB cap
		for i := range bigChunk {
			bigChunk[i] = 'x'
		}
		chunks = append(chunks, bigChunk)

		for i, chunk := range chunks {
			result := d.Process(chunk)
			if result.Truncate {
				t.Errorf("chunk %d: unexpected truncation for incremental app", i)
			}
		}

		if d.HasRecentFrame() {
			t.Error("HasRecentFrame should be false for app with no frames")
		}
	})

	t.Run("full-redraw app with frame boundaries - size cap works", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// First frame boundary
		result := d.Process([]byte("\x1b[2Jframe content"))
		if !result.Truncate {
			t.Error("expected truncation on screen clear")
		}

		if !d.HasRecentFrame() {
			t.Error("HasRecentFrame should be true after screen clear")
		}

		// Size cap should work because frame was recent
		bigChunk := make([]byte, 150*1024)
		result = d.Process(bigChunk)
		if !result.Truncate {
			t.Error("expected size cap truncation after recent frame")
		}
	})

	t.Run("hybrid app - frame at startup then incremental - size cap disabled after threshold", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// App starts with frame boundary (like btm)
		d.Process([]byte("\x1b[2Jinitial"))

		if !d.HasRecentFrame() {
			t.Error("HasRecentFrame should be true after frame")
		}

		// Send 210KB of incremental updates (exceeds 200KB recency window = MaxSize*2)
		chunk := make([]byte, 210*1024)
		d.Process(chunk)

		if d.HasRecentFrame() {
			t.Error("HasRecentFrame should be false after 210KB without frames")
		}

		// Now size cap should NOT apply even though buffer exceeds MaxSize
		bigChunk := make([]byte, 150*1024)
		result := d.Process(bigChunk)
		if result.Truncate {
			t.Error("size cap should NOT truncate when no recent frames")
		}
	})

	t.Run("continuous frames - size cap applies", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// Simulate app that sends frames frequently (like vim, htop)
		for i := 0; i < 5; i++ {
			d.Process([]byte("\x1b[2Jframe"))
			d.Process(make([]byte, 10*1024)) // 10KB between frames
		}

		if !d.HasRecentFrame() {
			t.Error("HasRecentFrame should be true with frequent frames")
		}

		// Size cap should work
		bigChunk := make([]byte, 150*1024)
		result := d.Process(bigChunk)
		if !result.Truncate {
			t.Error("expected size cap truncation with recent frames")
		}
	})

	t.Run("sync mode app - frame boundary detected on sync begin", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		result := d.Process([]byte("old\x1b[?2026hnew"))
		if !result.Truncate {
			t.Error("expected truncation on sync begin")
		}

		if !d.HasRecentFrame() {
			t.Error("HasRecentFrame should be true after sync begin")
		}
	})
}

func TestFrameDetector_Combined(t *testing.T) {
	tests := []struct {
		name      string
		strategy  TruncationStrategy
		chunks    [][]byte
		wantTrunc []bool
		wantData  [][]byte
	}{
		{
			name:      "screen clear wins in same chunk",
			strategy:  DefaultTUIStrategy(),
			chunks:    [][]byte{[]byte("old\x1b[0m\x1b[1;1H\x1b[2Jnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "sync and screen clear - last wins",
			strategy:  DefaultTUIStrategy(),
			chunks:    [][]byte{[]byte("old\x1b[?2026h\x1b[2Jnew")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("new")},
		},
		{
			name:      "all strategies combined",
			strategy:  DefaultTUIStrategy(),
			chunks:    [][]byte{[]byte("a\x1b[2Jb\x1b[?2026hc\x1b[0m\x1b[1;1Hd")},
			wantTrunc: []bool{true},
			wantData:  [][]byte{[]byte("d")},
		},
		{
			name: "disabled screen clear",
			strategy: TruncationStrategy{
				ScreenClear: false,
				SyncMode:    true,
			},
			chunks:    [][]byte{[]byte("old\x1b[2Jnew")},
			wantTrunc: []bool{false},
			wantData:  [][]byte{[]byte("old\x1b[2Jnew")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewFrameDetector(tt.strategy)

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.Truncate != tt.wantTrunc[i] {
					t.Errorf("chunk %d: Truncate = %v, want %v", i, result.Truncate, tt.wantTrunc[i])
				}

				if !bytes.Equal(result.DataAfter, tt.wantData[i]) {
					t.Errorf("chunk %d: DataAfter = %q, want %q", i, result.DataAfter, tt.wantData[i])
				}
			}
		})
	}
}

func TestFrameDetector_BufferSize(t *testing.T) {
	d := NewFrameDetector(TruncationStrategy{
		ScreenClear: true,
		MaxSize:     0,
	})

	// Initial buffer size is 0
	if d.BufferSize() != 0 {
		t.Errorf("initial BufferSize() = %d, want 0", d.BufferSize())
	}

	// Process some data
	d.Process([]byte("hello"))
	if d.BufferSize() != 5 {
		t.Errorf("after 'hello': BufferSize() = %d, want 5", d.BufferSize())
	}

	// Process more data
	d.Process([]byte(" world"))
	if d.BufferSize() != 11 {
		t.Errorf("after ' world': BufferSize() = %d, want 11", d.BufferSize())
	}

	// Clear resets size to data after clear
	d.Process([]byte("\x1b[2Jnew"))
	if d.BufferSize() != 3 {
		t.Errorf("after clear: BufferSize() = %d, want 3", d.BufferSize())
	}

	// Reset buffer size
	d.ResetBufferSize()
	if d.BufferSize() != 0 {
		t.Errorf("after reset: BufferSize() = %d, want 0", d.BufferSize())
	}
}

func TestFrameDetector_Reset(t *testing.T) {
	d := NewFrameDetector(DefaultTUIStrategy())

	// Build up state
	d.Process([]byte("\x1b[2Jframe content"))
	d.Process([]byte("more data"))

	if d.BufferSize() == 0 {
		t.Error("BufferSize should be > 0 before Reset")
	}
	if !d.HasRecentFrame() {
		t.Error("HasRecentFrame should be true before Reset")
	}

	d.Reset()

	if d.BufferSize() != 0 {
		t.Errorf("after Reset: BufferSize() = %d, want 0", d.BufferSize())
	}
	if d.HasRecentFrame() {
		t.Error("after Reset: HasRecentFrame should be false")
	}
	if d.BytesSinceLastFrame() != 0 {
		t.Errorf("after Reset: BytesSinceLastFrame() = %d, want 0", d.BytesSinceLastFrame())
	}

	// Flush should return nil (pending cleared)
	if pending := d.Flush(); pending != nil {
		t.Errorf("after Reset: Flush() = %q, want nil", pending)
	}

	// Detector should work normally after reset
	result := d.Process([]byte("\x1b[2Jnew frame"))
	if !result.Truncate {
		t.Error("after Reset: should detect truncation normally")
	}
	if string(result.DataAfter) != "new frame" {
		t.Errorf("after Reset: DataAfter = %q, want %q", result.DataAfter, "new frame")
	}
}

func TestDefaultTUIStrategy(t *testing.T) {
	s := DefaultTUIStrategy()

	if !s.ScreenClear {
		t.Error("ScreenClear should be enabled")
	}
	if !s.SyncMode {
		t.Error("SyncMode should be enabled")
	}
	if !s.CursorHome {
		t.Error("CursorHome should be enabled")
	}
	if !s.CursorJumpTop {
		t.Error("CursorJumpTop should be enabled")
	}
	if s.MaxSize != 100*1024 {
		t.Errorf("MaxSize = %d, want %d", s.MaxSize, 100*1024)
	}
}

func TestFrameDetector_RealWorldK9s(t *testing.T) {
	// k9s typically uses cursor home with reset for each frame
	d := NewFrameDetector(DefaultTUIStrategy())

	chunks := [][]byte{
		[]byte("frame1 content\n"),
		[]byte("\x1b[0m\x1b[1;1H"), // reset + cursor home
		[]byte("frame2 content\n"),
		[]byte("\x1b[0m\x1b[1;1H"), // reset + cursor home
		[]byte("frame3 content\n"),
	}

	var lastData []byte
	truncCount := 0

	for _, chunk := range chunks {
		result := d.Process(chunk)
		if result.Truncate {
			truncCount++
		}
		lastData = result.DataAfter
	}

	if truncCount != 2 {
		t.Errorf("truncCount = %d, want 2", truncCount)
	}

	if !bytes.Equal(lastData, []byte("frame3 content\n")) {
		t.Errorf("lastData = %q, want %q", lastData, "frame3 content\n")
	}
}

func TestFrameDetector_RealWorldClaudeCode(t *testing.T) {
	// Claude Code uses sync mode for updates
	// Truncation happens on sync BEGIN (h), keeping the frame content
	d := NewFrameDetector(DefaultTUIStrategy())

	chunks := [][]byte{
		[]byte("old"),
		[]byte("\x1b[?2026h"), // sync start - truncate here (new frame)
		[]byte("frame1"),
		[]byte("\x1b[?2026l"), // sync end - no truncate
		[]byte("\x1b[?2026h"), // sync start - truncate here (new frame)
		[]byte("frame2"),
		[]byte("\x1b[?2026l"), // sync end - no truncate
	}

	var lastData []byte
	truncCount := 0

	for _, chunk := range chunks {
		result := d.Process(chunk)
		if result.Truncate {
			truncCount++
		}
		lastData = result.DataAfter
	}

	if truncCount != 2 {
		t.Errorf("truncCount = %d, want 2", truncCount)
	}

	// Last data should be frame2 content plus the sync end
	if !bytes.Equal(lastData, []byte("\x1b[?2026l")) {
		t.Errorf("lastData = %q, want %q", lastData, "\x1b[?2026l")
	}
}

func TestFrameDetector_SizeCapWith4KBChunks(t *testing.T) {
	d := NewFrameDetector(DefaultTUIStrategy())

	// Send a frame boundary first
	result := d.Process([]byte("\x1b[2Jframe"))
	if !result.Truncate {
		t.Fatal("expected truncation on screen clear")
	}

	// Send ~30 x 4KB chunks (120KB total, exceeds 100KB MaxSize)
	// With recency window = MaxSize*2 = 200KB, the frame is still recent
	truncated := false
	for i := 0; i < 30; i++ {
		chunk := make([]byte, 4*1024)
		result := d.Process(chunk)
		if result.Truncate {
			truncated = true
			break
		}
	}

	if !truncated {
		t.Error("size cap should fire with default settings and 4KB chunks")
	}
}

func TestFrameDetector_RepeatedGrowthStaysBounded(t *testing.T) {
	d := NewFrameDetector(DefaultTUIStrategy())

	// Send initial frame
	d.Process([]byte("\x1b[2Jstart"))

	// First growth cycle: exceed MaxSize (100KB)
	firstCapFired := false
	for i := 0; i < 30; i++ {
		result := d.Process(make([]byte, 4*1024))
		if result.Truncate {
			firstCapFired = true
			break
		}
	}
	if !firstCapFired {
		t.Fatal("first size cap should have fired")
	}

	// Second growth cycle: after re-anchoring, cap should fire again
	secondCapFired := false
	for i := 0; i < 30; i++ {
		result := d.Process(make([]byte, 4*1024))
		if result.Truncate {
			secondCapFired = true
			break
		}
	}
	if !secondCapFired {
		t.Error("second size cap should fire after re-anchoring")
	}
}

func TestFrameDetector_CrossChunkCursorHomeHeuristic(t *testing.T) {
	t.Run("reset at end of chunk 1, cursor home at start of chunk 2", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// Chunk 1 ends with ESC[0m (reset)
		result := d.Process([]byte("old content\x1b[0m"))
		if result.Truncate {
			t.Error("chunk 1 should not truncate")
		}

		// Chunk 2 starts with ESC[1;1H (cursor home)
		result = d.Process([]byte("\x1b[1;1Hnew content"))
		if !result.Truncate {
			t.Error("chunk 2 should truncate (cross-chunk heuristic)")
		}
		if !bytes.Equal(result.DataAfter, []byte("new content")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "new content")
		}
	})

	t.Run("hide cursor at end of chunk 1, cursor home at start of chunk 2", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// Chunk 1 ends with ESC[?25l (hide cursor)
		result := d.Process([]byte("old content\x1b[?25l"))
		if result.Truncate {
			t.Error("chunk 1 should not truncate")
		}

		// Chunk 2 starts with ESC[1;1H (cursor home)
		result = d.Process([]byte("\x1b[1;1Hnew content"))
		if !result.Truncate {
			t.Error("chunk 2 should truncate (cross-chunk heuristic)")
		}
	})

	t.Run("heuristic marker too far back in previous chunk", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// Chunk 1: reset followed by >20 bytes of padding
		padding := make([]byte, 25)
		for i := range padding {
			padding[i] = 'x'
		}
		chunk1 := append([]byte("old\x1b[0m"), padding...)
		result := d.Process(chunk1)
		if result.Truncate {
			t.Error("chunk 1 should not truncate")
		}

		// Chunk 2: cursor home at start - heuristic marker is >20 bytes back, should NOT trigger
		result = d.Process([]byte("\x1b[1;1Hnew content"))
		if result.Truncate {
			t.Error("should NOT truncate when heuristic marker is beyond lookback window")
		}
	})
}

func TestFrameDetector_SnapshotMode(t *testing.T) {
	t.Run("sync mode suppressed during snapshot mode", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())
		d.SetSnapshotMode(true)

		result := d.Process([]byte("old\x1b[?2026hnew\x1b[?2026l"))
		if result.Truncate {
			t.Error("sync mode should NOT truncate during snapshot mode")
		}
		if !bytes.Equal(result.DataAfter, []byte("old\x1b[?2026hnew\x1b[?2026l")) {
			t.Errorf("DataAfter = %q, want full data", result.DataAfter)
		}
	})

	t.Run("screen clear suppressed during snapshot mode", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())
		d.SetSnapshotMode(true)

		result := d.Process([]byte("old\x1b[2Jnew"))
		if result.Truncate {
			t.Error("screen clear should NOT truncate during snapshot mode")
		}
		expected := []byte("old\x1b[2Jnew")
		if !bytes.Equal(result.DataAfter, expected) {
			t.Errorf("DataAfter = %q, want full data", result.DataAfter)
		}
	})

	t.Run("cursor home suppressed during snapshot mode", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())
		d.SetSnapshotMode(true)

		result := d.Process([]byte("old\x1b[0m\x1b[1;1Hnew"))
		if result.Truncate {
			t.Error("cursor_home should NOT truncate during snapshot mode")
		}
	})

	t.Run("CursorJumpTop suppressed during snapshot mode", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// Build up rows to exceed threshold
		var buf []byte
		for r := 1; r <= 30; r++ {
			buf = append(buf, []byte(fmt.Sprintf("\x1b[%d;1Hline content", r))...)
		}
		d.Process(buf)
		d.SetSnapshotMode(true)

		result := d.Process([]byte("\x1b[1;1Hnew frame"))
		if result.Truncate {
			t.Error("CursorJumpTop should NOT truncate during snapshot mode")
		}
	})

	t.Run("MaxSize suppressed during snapshot mode", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())

		// Trigger a frame boundary first (outside snapshot mode)
		d.Process([]byte("\x1b[2Jframe"))
		d.SetSnapshotMode(true)

		// Send data exceeding MaxSize (100KB)
		bigChunk := make([]byte, 150*1024)
		result := d.Process(bigChunk)
		if result.Truncate {
			t.Error("MaxSize should NOT truncate during snapshot mode")
		}
	})

	t.Run("vim redraw pattern: alt screen + clear + cursor home all suppressed in snapshot", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())
		d.SetSnapshotMode(true)

		// vim sends all three in one redraw sequence
		result := d.Process([]byte("\x1b[?1049h\x1b[2J\x1b[Hfile contents here"))
		if result.Truncate {
			t.Error("vim redraw pattern should NOT truncate during snapshot mode")
		}
		expected := []byte("\x1b[?1049h\x1b[2J\x1b[Hfile contents here")
		if !bytes.Equal(result.DataAfter, expected) {
			t.Errorf("DataAfter = %q, want full data", result.DataAfter)
		}
	})

	t.Run("sync mode resumes after snapshot mode disabled", func(t *testing.T) {
		d := NewFrameDetector(DefaultTUIStrategy())
		d.SetSnapshotMode(true)

		d.Process([]byte("data\x1b[?2026hmore"))
		d.SetSnapshotMode(false)

		result := d.Process([]byte("old\x1b[?2026hnew"))
		if !result.Truncate {
			t.Error("sync mode should truncate after snapshot mode disabled")
		}
		if !bytes.Equal(result.DataAfter, []byte("new")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "new")
		}
	})
}

func TestFrameDetector_CursorJumpTop(t *testing.T) {
	// Helper: build cursor position sequence ESC[row;1H
	cursorPos := func(row int) string {
		return fmt.Sprintf("\x1b[%d;1H", row)
	}

	t.Run("jump from row 30 to row 1 triggers truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		// Build up rows to exceed threshold (10)
		var buf []byte
		for r := 1; r <= 30; r++ {
			buf = append(buf, []byte(cursorPos(r)+"line content")...)
		}
		result := d.Process(buf)
		if result.Truncate {
			t.Error("building up rows should not truncate")
		}

		// Now jump back to row 1
		result = d.Process([]byte(cursorPos(1) + "new frame"))
		if !result.Truncate {
			t.Error("jump from row 30 to row 1 should truncate")
		}
		if !bytes.Equal(result.DataAfter, []byte("new frame")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "new frame")
		}
	})

	t.Run("jump to row 2 also triggers truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		var buf []byte
		for r := 1; r <= 20; r++ {
			buf = append(buf, []byte(cursorPos(r)+"content")...)
		}
		d.Process(buf)

		result := d.Process([]byte(cursorPos(2) + "new frame"))
		if !result.Truncate {
			t.Error("jump to row 2 should also truncate")
		}
	})

	t.Run("sequential rows do not trigger truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		// Write rows 1..20 sequentially in one chunk
		var buf []byte
		for r := 1; r <= 20; r++ {
			buf = append(buf, []byte(cursorPos(r)+"content")...)
		}
		result := d.Process(buf)
		if result.Truncate {
			t.Error("sequential rows should not truncate")
		}
	})

	t.Run("max row below threshold does not truncate even on jump", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		// Only go up to row 5 (below threshold of 10)
		var buf []byte
		for r := 1; r <= 5; r++ {
			buf = append(buf, []byte(cursorPos(r)+"content")...)
		}
		d.Process(buf)

		result := d.Process([]byte(cursorPos(1) + "new frame"))
		if result.Truncate {
			t.Error("should not truncate when maxRowSeen < threshold")
		}
	})

	t.Run("strategy disabled does not truncate", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: false})

		var buf []byte
		for r := 1; r <= 30; r++ {
			buf = append(buf, []byte(cursorPos(r)+"content")...)
		}
		d.Process(buf)

		result := d.Process([]byte(cursorPos(1) + "new frame"))
		if result.Truncate {
			t.Error("should not truncate when CursorJumpTop is disabled")
		}
	})

	t.Run("cross-chunk row tracking persists", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		// Chunk 1: rows 1..15
		var buf1 []byte
		for r := 1; r <= 15; r++ {
			buf1 = append(buf1, []byte(cursorPos(r)+"content")...)
		}
		result := d.Process(buf1)
		if result.Truncate {
			t.Error("chunk 1 should not truncate")
		}

		// Chunk 2: rows 16..25
		var buf2 []byte
		for r := 16; r <= 25; r++ {
			buf2 = append(buf2, []byte(cursorPos(r)+"content")...)
		}
		result = d.Process(buf2)
		if result.Truncate {
			t.Error("chunk 2 should not truncate")
		}

		// Chunk 3: jump back to row 1
		result = d.Process([]byte(cursorPos(1) + "new frame"))
		if !result.Truncate {
			t.Error("cross-chunk jump should truncate")
		}
	})

	t.Run("cross-chunk partial cursor position sequence", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		// Build up rows first
		var buf []byte
		for r := 1; r <= 20; r++ {
			buf = append(buf, []byte(cursorPos(r)+"content")...)
		}
		d.Process(buf)

		// Send partial sequence: ESC[1;1 (missing H)
		result := d.Process([]byte("data\x1b[1;1"))
		if result.Truncate {
			t.Error("partial sequence should not truncate yet")
		}

		// Complete the sequence in next chunk
		result = d.Process([]byte("Hnew frame"))
		if !result.Truncate {
			t.Error("completed cross-chunk cursor position should truncate")
		}
	})
}

func TestFrameDetector_CursorHomeCooldown(t *testing.T) {
	t.Run("vim pattern: two cursor_home within 4096 bytes - only first fires", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// Single chunk with two cursor_home sequences close together
		// First: reset + cursor_home
		// Then some content (< 4096 bytes)
		// Second: reset + cursor_home
		padding := make([]byte, 200)
		for i := range padding {
			padding[i] = 'x'
		}
		var chunk []byte
		chunk = append(chunk, []byte("old\x1b[?25l\x1b[1;1H")...) // first cursor_home
		chunk = append(chunk, padding...)
		chunk = append(chunk, []byte("\x1b[0m\x1b[1;1H")...) // second cursor_home (within cooldown)
		chunk = append(chunk, []byte("final content")...)

		result := d.Process(chunk)
		if !result.Truncate {
			t.Error("should truncate (first cursor_home)")
		}
		// The truncation should be at the first cursor_home, not the second
		// (second is suppressed by cooldown)
		expected := append([]byte{}, padding...)
		expected = append(expected, []byte("\x1b[0m\x1b[1;1Hfinal content")...)
		if !bytes.Equal(result.DataAfter, expected) {
			t.Errorf("DataAfter should include data after first cursor_home (second suppressed)")
		}
	})

	t.Run("normal multi-frame: two cursor_home 5000+ bytes apart - both fire", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		padding := make([]byte, 5000)
		for i := range padding {
			padding[i] = 'x'
		}
		var chunk []byte
		chunk = append(chunk, []byte("old\x1b[0m\x1b[1;1H")...) // first cursor_home
		chunk = append(chunk, padding...)
		chunk = append(chunk, []byte("\x1b[0m\x1b[1;1H")...) // second cursor_home (beyond cooldown)
		chunk = append(chunk, []byte("final")...)

		result := d.Process(chunk)
		if !result.Truncate {
			t.Error("should truncate")
		}
		// Both should fire, so DataAfter should be after the second cursor_home
		if !bytes.Equal(result.DataAfter, []byte("final")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "final")
		}
	})

	t.Run("separate chunks: cursor_home in different Process calls both fire", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		result := d.Process([]byte("old\x1b[0m\x1b[1;1Hframe1"))
		if !result.Truncate {
			t.Error("first chunk should truncate")
		}

		result = d.Process([]byte("data\x1b[0m\x1b[1;1Hframe2"))
		if !result.Truncate {
			t.Error("second chunk should truncate (cooldown resets between Process calls)")
		}
	})

	t.Run("Reset clears cooldown state", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		d.Process([]byte("old\x1b[0m\x1b[1;1Hcontent"))
		d.Reset()

		result := d.Process([]byte("new\x1b[0m\x1b[1;1Hcontent"))
		if !result.Truncate {
			t.Error("after Reset, cursor_home should work normally")
		}
	})
}

func TestFrameDetector_CursorJumpTopLookAhead(t *testing.T) {
	cursorPos := func(row int) string {
		return fmt.Sprintf("\x1b[%d;1H", row)
	}

	buildRows := func(from, to int) []byte {
		var buf []byte
		for r := from; r <= to; r++ {
			buf = append(buf, []byte(cursorPos(r)+"line content")...)
		}
		return buf
	}

	t.Run("micro pattern: jump to top followed by cursor control only - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 40))

		// Jump to row 1 followed by cursor control sequences only (no printable content)
		result := d.Process([]byte(cursorPos(1) + "\x1b[?12l\x1b[?25h"))
		if result.Truncate {
			t.Error("should NOT truncate when only cursor control follows jump")
		}
	})

	t.Run("htop pattern: jump to top followed by content - truncation fires", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 30))

		result := d.Process([]byte(cursorPos(1) + "CPU usage 50%"))
		if !result.Truncate {
			t.Error("should truncate when content follows jump")
		}
		if !bytes.Equal(result.DataAfter, []byte("CPU usage 50%")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "CPU usage 50%")
		}
	})

	t.Run("jump followed by color then content - truncation fires", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 20))

		result := d.Process([]byte(cursorPos(1) + "\x1b[32mgreen text"))
		if !result.Truncate {
			t.Error("should truncate when content follows after color sequences")
		}
	})

	t.Run("cross-chunk: jump at end of chunk, content in next - truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 25))

		// Jump at very end of chunk (near boundary)
		result := d.Process([]byte(cursorPos(1)))
		if result.Truncate {
			t.Error("should not truncate yet (waiting for next chunk)")
		}

		// Next chunk has content
		result = d.Process([]byte("new frame content"))
		if !result.Truncate {
			t.Error("should truncate when content arrives in next chunk")
		}
	})

	t.Run("cross-chunk: jump at end of chunk, cursor control in next - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 25))

		// Jump at end of chunk
		result := d.Process([]byte(cursorPos(1)))
		if result.Truncate {
			t.Error("should not truncate yet")
		}

		// Next chunk has only cursor control
		result = d.Process([]byte("\x1b[?25h\x1b[?12l"))
		if result.Truncate {
			t.Error("should NOT truncate when only cursor control in next chunk")
		}
	})

	t.Run("jump followed by another cursor positioning - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 20))

		// Jump to row 1 followed by another cursor position (row 5) with no content between
		result := d.Process([]byte("\x1b[1;3H\x1b[?12l\x1b[?25h"))
		if result.Truncate {
			t.Error("should NOT truncate when only cursor positioning/control follows")
		}
	})

	t.Run("jump followed by tilde-terminated CSI then content - truncation fires", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorJumpTop: true})

		d.Process(buildRows(1, 20))

		result := d.Process([]byte(cursorPos(1) + "\x1b[15~hello world"))
		if !result.Truncate {
			t.Error("should truncate when content follows after tilde-terminated sequence")
		}
	})
}

func TestFrameDetector_CursorHomeLookAhead(t *testing.T) {
	t.Run("vim pattern: second cursor_home followed by control only - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// First cursor_home with content after (legitimate frame start)
		// Then ~5000 bytes of row content
		// Then second cursor_home followed by only cursor control (editing cursor return)
		padding := make([]byte, 5000)
		for i := range padding {
			padding[i] = 'x'
		}
		var chunk []byte
		chunk = append(chunk, []byte("\x1b[?25l\x1b[1;1H")...) // first cursor_home (frame start)
		chunk = append(chunk, padding...)
		chunk = append(chunk, []byte("\x1b[0m\x1b[1;1H")...)     // second cursor_home (editing cursor)
		chunk = append(chunk, []byte("\x1b[?12l\x1b[?25h")...)   // cursor control only

		result := d.Process(chunk)
		if !result.Truncate {
			t.Error("should truncate (first cursor_home)")
		}
		// DataAfter should be from first cursor_home, second should be suppressed
		wantLen := 5000 + len("\x1b[0m\x1b[1;1H") + len("\x1b[?12l\x1b[?25h")
		if len(result.DataAfter) != wantLen {
			t.Errorf("DataAfter len = %d, want %d (second cursor_home should be suppressed)", len(result.DataAfter), wantLen)
		}
	})

	t.Run("micro pattern: cursor_home followed by cursor control only - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// cursor_home with heuristic, followed by only cursor control sequences
		result := d.Process([]byte("content\x1b[0m\x1b[1;1H\x1b[?12l\x1b[?25h"))
		if result.Truncate {
			t.Error("should NOT truncate when only cursor control follows cursor_home")
		}
	})

	t.Run("multi-frame with content still works: both fire", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		padding := make([]byte, 5000)
		for i := range padding {
			padding[i] = 'x'
		}
		var chunk []byte
		chunk = append(chunk, []byte("old\x1b[0m\x1b[1;1H")...) // first cursor_home
		chunk = append(chunk, padding...)
		chunk = append(chunk, []byte("\x1b[0m\x1b[1;1H")...) // second cursor_home (beyond cooldown)
		chunk = append(chunk, []byte("final")...)             // content after second

		result := d.Process(chunk)
		if !result.Truncate {
			t.Error("should truncate")
		}
		// Both should fire, DataAfter from second cursor_home
		if !bytes.Equal(result.DataAfter, []byte("final")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "final")
		}
	})

	t.Run("cross-chunk deferred: content follows - truncation fires", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// cursor_home at the very end of chunk (no bytes after)
		result := d.Process([]byte("old\x1b[0m\x1b[1;1H"))
		if result.Truncate {
			t.Error("should not truncate yet (deferred)")
		}

		// Next chunk has content
		result = d.Process([]byte("new frame content"))
		if !result.Truncate {
			t.Error("should truncate when content arrives in next chunk")
		}
		if !bytes.Equal(result.DataAfter, []byte("new frame content")) {
			t.Errorf("DataAfter = %q, want %q", result.DataAfter, "new frame content")
		}
	})

	t.Run("cross-chunk deferred: control only - no truncation", func(t *testing.T) {
		d := NewFrameDetector(TruncationStrategy{CursorHome: true})

		// cursor_home at the very end of chunk
		result := d.Process([]byte("old\x1b[0m\x1b[1;1H"))
		if result.Truncate {
			t.Error("should not truncate yet (deferred)")
		}

		// Next chunk has only cursor control
		result = d.Process([]byte("\x1b[?25h\x1b[?12l"))
		if result.Truncate {
			t.Error("should NOT truncate when only cursor control in next chunk")
		}
	})
}

func TestFrameDetector_ConcurrentAccess(t *testing.T) {
	d := NewFrameDetector(DefaultTUIStrategy())
	var wg sync.WaitGroup

	wg.Add(3)

	// Goroutine 1: continuous Process calls
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.Process([]byte("\x1b[2Jframe data"))
		}
	}()

	// Goroutine 2: SetSnapshotMode toggles
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.SetSnapshotMode(true)
			d.SetSnapshotMode(false)
		}
	}()

	// Goroutine 3: Reset calls
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.Reset()
		}
	}()

	wg.Wait()
}

func TestParseCursorRow_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		pos     int
		wantRow int
		wantEnd int
		wantOK  bool
	}{
		{
			name:    "ESC[H defaults to row 1",
			data:    []byte("\x1b[H"),
			pos:     0,
			wantRow: 1,
			wantEnd: 3,
			wantOK:  true,
		},
		{
			name:    "ESC[;5H no row before semicolon defaults to row 1",
			data:    []byte("\x1b[;5H"),
			pos:     0,
			wantRow: 1,
			wantEnd: 5,
			wantOK:  true,
		},
		{
			name:    "ESC[0;0H zero row",
			data:    []byte("\x1b[0;0H"),
			pos:     0,
			wantRow: 0,
			wantEnd: 6,
			wantOK:  true,
		},
		{
			name:   "very large row number (6 digits) rejected",
			data:   []byte("\x1b[123456;1H"),
			pos:    0,
			wantOK: false,
		},
		{
			name:    "5 digit row number accepted",
			data:    []byte("\x1b[99999;1H"),
			pos:     0,
			wantRow: 99999,
			wantEnd: 10,
			wantOK:  true,
		},
		{
			name:   "very large col number (6 digits) rejected",
			data:   []byte("\x1b[1;123456H"),
			pos:    0,
			wantOK: false,
		},
		{
			name:   "incomplete sequence",
			data:   []byte("\x1b[5"),
			pos:    0,
			wantOK: false,
		},
		{
			name:   "not a cursor position (color code)",
			data:   []byte("\x1b[31m"),
			pos:    0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, end, ok := parseCursorRow(tt.data, tt.pos)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if row != tt.wantRow {
				t.Errorf("row = %d, want %d", row, tt.wantRow)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}
