package vterm

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/x/vt"
)

// Screen wraps a thread-safe VT emulator for TUI session handling.
// It replaces FrameDetector + TerminalResponder + raw byte storage for TUI sessions.
// The emulator IS the screen state; no separate buffer needed.
type Screen struct {
	emu     *vt.SafeEmulator
	version atomic.Uint64

	// Response bridge: internal goroutine reads from emu.Read() and writes
	// to respPW. ReadResponses reads from respPR. This avoids a data race
	// in the charmbracelet library between emu.Read() and emu.Close().
	respPR     *io.PipeReader
	respPW     *io.PipeWriter
	bridgeDone chan struct{}
	closeOnce  sync.Once
}

func New(cols, rows int) *Screen {
	pr, pw := io.Pipe()
	s := &Screen{
		emu:        vt.NewSafeEmulator(cols, rows),
		respPR:     pr,
		respPW:     pw,
		bridgeDone: make(chan struct{}),
	}
	go s.bridgeResponses()
	return s
}

func (s *Screen) Write(p []byte) (int, error) {
	n, err := s.emu.Write(p)
	if n > 0 {
		s.version.Add(1)
	}
	return n, err
}

// String returns plain text screen content with \r\n normalized to \n
// and trailing empty lines removed.
func (s *Screen) String() string {
	out := s.emu.String()
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "")
	return trimTrailingEmptyLines(out)
}

// Render returns ANSI-styled screen content with \r\n normalized to \n.
func (s *Screen) Render() string {
	out := s.emu.Render()
	out = strings.ReplaceAll(out, "\r\n", "\n")
	out = strings.ReplaceAll(out, "\r", "")
	return out
}

func (s *Screen) Resize(cols, rows int) {
	s.emu.Resize(cols, rows)
}

func (s *Screen) Version() uint64 {
	return s.version.Load()
}

func (s *Screen) Close() error {
	s.closeOnce.Do(func() {
		s.respPR.Close()

		// Close the emulator's internal pipe writer via InputPipe() to unblock
		// bridgeResponses (emu.Read returns EOF). This avoids a data race in the
		// charmbracelet library where emu.Close() modifies the pipe reader field
		// concurrently with emu.Read() accessing it.
		if pw, ok := s.emu.InputPipe().(io.Closer); ok {
			pw.Close()
		}
		<-s.bridgeDone

		// bridgeResponses has exited, no concurrent Read(), safe to close
		s.emu.Close()
	})
	return nil
}

// bridgeResponses reads terminal query responses from the emulator's internal
// pipe and forwards them to our intermediate pipe. Runs as a goroutine started
// in New(). Exits when the emulator is closed.
func (s *Screen) bridgeResponses() {
	defer close(s.bridgeDone)
	buf := make([]byte, 1024)
	for {
		n, err := s.emu.Read(buf)
		if n > 0 {
			if _, werr := s.respPW.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			s.respPW.CloseWithError(err)
			return
		}
	}
}

// ReadResponses reads terminal query responses and writes them to w
// (typically the PTY master). Run as a goroutine. Exits when the
// screen is closed.
func (s *Screen) ReadResponses(w io.Writer) {
	buf := make([]byte, 1024)
	for {
		n, err := s.respPR.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func trimTrailingEmptyLines(s string) string {
	lines := strings.Split(s, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimRight(lines[last], " ") == "" {
		last--
	}
	if last < 0 {
		return ""
	}
	return strings.Join(lines[:last+1], "\n")
}
