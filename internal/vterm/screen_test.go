package vterm

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestScreen_WriteAndString(t *testing.T) {
	s := New(80, 24)
	defer s.Close()

	s.Write([]byte("hello world"))
	got := s.String()
	if !strings.Contains(got, "hello world") {
		t.Errorf("String() = %q, want to contain 'hello world'", got)
	}
}

func TestScreen_VersionIncrements(t *testing.T) {
	s := New(80, 24)
	defer s.Close()

	v0 := s.Version()
	s.Write([]byte("first"))
	v1 := s.Version()
	s.Write([]byte("second"))
	v2 := s.Version()

	if v1 <= v0 {
		t.Errorf("version should increase after first write: v0=%d, v1=%d", v0, v1)
	}
	if v2 <= v1 {
		t.Errorf("version should increase after second write: v1=%d, v2=%d", v1, v2)
	}
}

func TestScreen_Resize(t *testing.T) {
	s := New(40, 10)
	defer s.Close()

	s.Write([]byte("\x1b[1;1Hnarrow"))
	s.Resize(120, 40)
	s.Write([]byte("\x1b[1;1Hwide content here"))

	got := s.String()
	if !strings.Contains(got, "wide content here") {
		t.Errorf("String() after resize = %q, want to contain 'wide content here'", got)
	}
}

func TestScreen_ReadResponses_DA1(t *testing.T) {
	s := New(80, 24)

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		s.ReadResponses(&buf)
		close(done)
	}()

	// Send DA1 query (ESC[c) which triggers a response
	s.Write([]byte("\x1b[c"))

	// Give response bridge time to forward, then close to unblock Read
	time.Sleep(50 * time.Millisecond)
	s.Close()
	<-done

	resp := buf.String()
	if !strings.Contains(resp, "\x1b[?") {
		t.Errorf("ReadResponses did not bridge DA1 response: got %q", resp)
	}
}

func TestScreen_EmptyString(t *testing.T) {
	s := New(80, 24)
	defer s.Close()
	got := s.String()
	if got != "" {
		t.Errorf("empty screen String() = %q, want empty", got)
	}
}

func TestScreen_TUIContent(t *testing.T) {
	s := New(80, 24)
	defer s.Close()

	// Simulate TUI app drawing with cursor positioning
	s.Write([]byte("\x1b[1;1HHeader Line\x1b[2;1HContent Row\x1b[3;1HFooter"))
	got := s.String()
	if !strings.Contains(got, "Header Line") {
		t.Errorf("String() = %q, want to contain 'Header Line'", got)
	}
	if !strings.Contains(got, "Content Row") {
		t.Errorf("String() = %q, want to contain 'Content Row'", got)
	}
	if !strings.Contains(got, "Footer") {
		t.Errorf("String() = %q, want to contain 'Footer'", got)
	}
}

func TestScreen_RenderContainsANSI(t *testing.T) {
	s := New(80, 24)
	defer s.Close()

	s.Write([]byte("\x1b[31mred text\x1b[0m"))
	rendered := s.Render()
	if !strings.Contains(rendered, "red text") {
		t.Errorf("Render() = %q, want to contain 'red text'", rendered)
	}
}
