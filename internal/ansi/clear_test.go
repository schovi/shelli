package ansi

import (
	"bytes"
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
			d := NewScreenClearDetector()

			for i, chunk := range tt.chunks {
				result := d.Process(chunk)

				if result.ClearFound != tt.wantClears[i] {
					t.Errorf("chunk %d: ClearFound = %v, want %v", i, result.ClearFound, tt.wantClears[i])
				}

				if !bytes.Equal(result.DataAfter, tt.wantData[i]) {
					t.Errorf("chunk %d: DataAfter = %q, want %q", i, result.DataAfter, tt.wantData[i])
				}
			}
		})
	}
}

func TestScreenClearDetector_Flush(t *testing.T) {
	d := NewScreenClearDetector()

	// Send chunk ending with partial escape sequence
	result := d.Process([]byte("data\x1b"))
	if result.ClearFound {
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
	d := NewScreenClearDetector()

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
		if result.ClearFound {
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
