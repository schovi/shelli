package ansi

import (
	"os"
	"testing"
	"time"
)

func readPipe(r *os.File, timeout time.Duration) []byte {
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		done <- buf[:n]
	}()

	select {
	case data := <-done:
		return data
	case <-time.After(timeout):
		return nil
	}
}

func TestTerminalResponder_DA1(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	resp := NewTerminalResponder(w)

	t.Run("ESC[c", func(t *testing.T) {
		result := resp.Process([]byte("before\x1b[cafter"))
		if string(result) != "beforeafter" {
			t.Errorf("result = %q, want %q", result, "beforeafter")
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[?62;1;2;6;7;8;9;15;22c"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})

	t.Run("ESC[0c", func(t *testing.T) {
		result := resp.Process([]byte("\x1b[0c"))
		if string(result) != "" {
			t.Errorf("result = %q, want empty", result)
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[?62;1;2;6;7;8;9;15;22c"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})
}

func TestTerminalResponder_DA2(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	resp := NewTerminalResponder(w)

	t.Run("ESC[>c", func(t *testing.T) {
		result := resp.Process([]byte("\x1b[>c"))
		if string(result) != "" {
			t.Errorf("result = %q, want empty", result)
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[>1;1;0c"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})

	t.Run("ESC[>0c", func(t *testing.T) {
		result := resp.Process([]byte("\x1b[>0c"))
		if string(result) != "" {
			t.Errorf("result = %q, want empty", result)
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[>1;1;0c"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})
}

func TestTerminalResponder_KittyKeyboard(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	resp := NewTerminalResponder(w)

	result := resp.Process([]byte("\x1b[?u"))
	if string(result) != "" {
		t.Errorf("result = %q, want empty", result)
	}
	got := readPipe(r, 100*time.Millisecond)
	expected := "\x1b[?0u"
	if string(got) != expected {
		t.Errorf("response = %q, want %q", got, expected)
	}
}

func TestTerminalResponder_DECRPM(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	resp := NewTerminalResponder(w)

	t.Run("mode 2004", func(t *testing.T) {
		result := resp.Process([]byte("\x1b[?2004$p"))
		if string(result) != "" {
			t.Errorf("result = %q, want empty", result)
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[?2004;0$y"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})

	t.Run("mode 25", func(t *testing.T) {
		result := resp.Process([]byte("\x1b[?25$p"))
		if string(result) != "" {
			t.Errorf("result = %q, want empty", result)
		}
		got := readPipe(r, 100*time.Millisecond)
		expected := "\x1b[?25;0$y"
		if string(got) != expected {
			t.Errorf("response = %q, want %q", got, expected)
		}
	})
}

func TestTerminalResponder_NoQueries(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	resp := NewTerminalResponder(w)

	input := []byte("hello world\x1b[31mred\x1b[0m")
	result := resp.Process(input)
	if string(result) != string(input) {
		t.Errorf("result = %q, want %q", result, input)
	}
}

func TestTerminalResponder_MultipleQueries(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	resp := NewTerminalResponder(w)

	// DA1 + Kitty in one chunk
	result := resp.Process([]byte("text\x1b[c\x1b[?umore"))
	if string(result) != "textmore" {
		t.Errorf("result = %q, want %q", result, "textmore")
	}

	// Read all responses (may arrive in one or multiple reads)
	expected := "\x1b[?62;1;2;6;7;8;9;15;22c" + "\x1b[?0u"
	var all []byte
	for len(all) < len(expected) {
		got := readPipe(r, 200*time.Millisecond)
		if got == nil {
			break
		}
		all = append(all, got...)
	}

	if string(all) != expected {
		t.Errorf("combined responses = %q, want %q", all, expected)
	}
}

func TestTerminalResponder_NoESCFastPath(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	resp := NewTerminalResponder(w)

	input := []byte("plain text without any escape sequences")
	result := resp.Process(input)
	// Fast path should return the same slice
	if string(result) != string(input) {
		t.Errorf("result = %q, want %q", result, input)
	}
}

