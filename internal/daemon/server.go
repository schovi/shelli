package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/schovi/shelli/internal/ansi"
)

type ptyHandle struct {
	f         *os.File
	closeOnce sync.Once
}

func (p *ptyHandle) Close() {
	p.closeOnce.Do(func() {
		p.f.Close()
	})
}

func (p *ptyHandle) File() *os.File {
	return p.f
}

type SessionInfo struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
	State     string `json:"state"`
	StoppedAt string `json:"stopped_at,omitempty"`
}

type sessionHandle struct {
	name      string
	pid       int
	command   string
	state     SessionState
	createdAt time.Time
	stoppedAt *time.Time

	pty           *ptyHandle
	cmd           *exec.Cmd
	done          chan struct{}
	frameDetector *ansi.FrameDetector
	responder     *ansi.TerminalResponder
}

type Server struct {
	mu      sync.Mutex
	handles map[string]*sessionHandle

	socketDir string
	storage   OutputStorage
	listener  net.Listener

	stoppedTTL      time.Duration
	cleanupStopChan chan struct{}
}

type ServerOption func(*Server)

func WithStorage(storage OutputStorage) ServerOption {
	return func(s *Server) {
		s.storage = storage
	}
}

func WithStoppedTTL(ttl time.Duration) ServerOption {
	return func(s *Server) {
		s.stoppedTTL = ttl
	}
}

func WithSocketDir(dir string) ServerOption {
	return func(s *Server) {
		s.socketDir = dir
	}
}

// Deprecated: use WithStorage instead
func WithMaxOutputSize(size int) ServerOption {
	return func(s *Server) {
		if mem, ok := s.storage.(*MemoryStorage); ok {
			mem.maxOutputSize = size
		}
	}
}

func NewServer(opts ...ServerOption) (*Server, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	socketDir := filepath.Join(homeDir, ".shelli")
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return nil, err
	}

	s := &Server{
		handles:         make(map[string]*sessionHandle),
		socketDir:       socketDir,
		storage:         NewMemoryStorage(DefaultMaxOutputSize),
		cleanupStopChan: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	if err := s.recoverSessions(); err != nil {
		return nil, fmt.Errorf("recover sessions: %w", err)
	}

	return s, nil
}

func (s *Server) recoverSessions() error {
	sessions, err := s.storage.ListSessions()
	if err != nil {
		return err
	}

	for _, name := range sessions {
		meta, err := s.storage.LoadMeta(name)
		if err != nil {
			continue
		}

		if meta.State == StateRunning {
			meta.State = StateStopped
			now := time.Now()
			meta.StoppedAt = &now
			s.storage.SaveMeta(name, meta)
		}

		s.handles[name] = &sessionHandle{
			name:      meta.Name,
			pid:       meta.PID,
			command:   meta.Command,
			state:     meta.State,
			createdAt: meta.CreatedAt,
			stoppedAt: meta.StoppedAt,
		}
	}

	return nil
}

func SocketPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".shelli", "shelli.sock")
}

func (s *Server) socketPath() string {
	return filepath.Join(s.socketDir, "shelli.sock")
}

func (s *Server) Start() error {
	sockPath := s.socketPath()
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = listener

	if s.stoppedTTL > 0 {
		go s.runCleanup()
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			isShutdown := s.listener == nil
			s.mu.Unlock()
			if isShutdown {
				return nil
			}
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) runCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.cleanupStopChan:
			return
		case <-ticker.C:
			s.cleanupExpiredSessions()
		}
	}
}

func (s *Server) cleanupExpiredSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for name, h := range s.handles {
		if h.state == StateStopped && h.stoppedAt != nil {
			if now.Sub(*h.stoppedAt) > s.stoppedTTL {
				s.storage.Delete(name)
				delete(s.handles, name)
			}
		}
	}
}

func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.cleanupStopChan)

	for _, h := range s.handles {
		if h.state == StateRunning {
			if h.done != nil {
				close(h.done)
			}
			if h.pty != nil {
				h.pty.Close()
			}
			if h.cmd != nil {
				h.cmd.Process.Kill()
				h.cmd.Wait()
			}
		}
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	os.Remove(s.socketPath())
}

type Request struct {
	Version    int      `json:"version,omitempty"`
	Action     string   `json:"action"`
	Name       string   `json:"name,omitempty"`
	Command    string   `json:"command,omitempty"`
	Input      string   `json:"input,omitempty"`
	Newline    bool     `json:"newline,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	HeadLines  int      `json:"head_lines,omitempty"`
	TailLines  int      `json:"tail_lines,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
	Pattern    string   `json:"pattern,omitempty"`
	Before     int      `json:"before,omitempty"`
	After      int      `json:"after,omitempty"`
	IgnoreCase bool     `json:"ignore_case,omitempty"`
	StripANSI  bool     `json:"strip_ansi,omitempty"`
	Cols       int      `json:"cols,omitempty"`
	Rows       int      `json:"rows,omitempty"`
	Env        []string `json:"env,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	TUIMode    bool     `json:"tui_mode,omitempty"`
	Snapshot   bool     `json:"snapshot,omitempty"`
	SettleMs   int      `json:"settle_ms,omitempty"`
	TimeoutSec int      `json:"timeout_sec,omitempty"`
}

type Response struct {
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in handleConn: %v\n%s", r, debug.Stack())
		}
	}()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		s.sendResponse(conn, Response{Success: false, Error: err.Error()})
		return
	}

	if req.Version != ProtocolVersion && req.Version != 0 {
		s.sendResponse(conn, Response{
			Success: false,
			Error: fmt.Sprintf("protocol version mismatch: client=%d, daemon=%d. Restart daemon with: shelli daemon --stop && shelli daemon", req.Version, ProtocolVersion),
		})
		return
	}

	var resp Response
	switch req.Action {
	case "create":
		resp = s.handleCreate(req)
	case "list":
		resp = s.handleList()
	case "read":
		resp = s.handleRead(req)
	case "send":
		resp = s.handleSend(req)
	case "stop":
		resp = s.handleStop(req)
	case "kill":
		resp = s.handleKill(req)
	case "search":
		resp = s.handleSearch(req)
	case "info":
		resp = s.handleInfo(req)
	case "clear":
		resp = s.handleClear(req)
	case "resize":
		resp = s.handleResize(req)
	case "size":
		resp = s.handleSize(req)
	case "ping":
		resp = Response{Success: true, Data: "pong"}
	default:
		resp = Response{Success: false, Error: "unknown action"}
	}

	s.sendResponse(conn, resp)
}

func (s *Server) sendResponse(conn net.Conn, resp Response) {
	json.NewEncoder(conn).Encode(resp)
}

func (s *Server) handleCreate(req Request) Response {
	if err := ValidateSessionName(req.Name); err != nil {
		return Response{Success: false, Error: err.Error()}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.handles[req.Name]; exists {
		return Response{Success: false, Error: fmt.Sprintf("session %q already exists", req.Name)}
	}

	command := req.Command
	if command == "" {
		command = os.Getenv("SHELL")
		if command == "" {
			command = "/bin/sh"
		}
	}

	var cmd *exec.Cmd
	if strings.Contains(command, " ") {
		cmd = exec.Command("sh", "-c", command) // #nosec G702 -- executing user-provided commands is the core feature
	} else {
		cmd = exec.Command(command) // #nosec G702 -- executing user-provided commands is the core feature
	}

	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Env = append(cmd.Env, req.Env...)

	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}

	cols := req.Cols
	if cols <= 0 {
		cols = 80
	}
	rows := req.Rows
	if rows <= 0 {
		rows = 24
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("start pty: %v", err)}
	}

	now := time.Now()
	meta := &SessionMeta{
		Name:      req.Name,
		Command:   command,
		PID:       cmd.Process.Pid,
		State:     StateRunning,
		CreatedAt: now,
		ReadPos:   0,
		Cols:      cols,
		Rows:      rows,
		TUIMode:   req.TUIMode,
	}

	if err := s.storage.Create(req.Name, meta); err != nil {
		ptmx.Close()
		cmd.Process.Kill()
		return Response{Success: false, Error: fmt.Sprintf("create storage: %v", err)}
	}

	h := &sessionHandle{
		name:      req.Name,
		pid:       cmd.Process.Pid,
		command:   command,
		state:     StateRunning,
		createdAt: now,
		pty:       &ptyHandle{f: ptmx},
		cmd:       cmd,
		done:      make(chan struct{}),
	}
	if req.TUIMode {
		h.frameDetector = ansi.NewFrameDetector(ansi.DefaultTUIStrategy())
		h.responder = ansi.NewTerminalResponder(ptmx)
	}

	s.handles[req.Name] = h

	go s.captureOutput(req.Name, h)

	return Response{Success: true, Data: map[string]interface{}{
		"name":       h.name,
		"pid":        h.pid,
		"command":    h.command,
		"created_at": h.createdAt,
		"cols":       cols,
		"rows":       rows,
	}}
}

func (s *Server) captureOutput(name string, h *sessionHandle) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in captureOutput[%s]: %v\n%s", name, r, debug.Stack())
		}
	}()

	s.mu.Lock()
	done := h.done
	p := h.pty
	cmd := h.cmd
	detector := h.frameDetector
	responder := h.responder
	storage := s.storage
	s.mu.Unlock()

	if p == nil {
		return
	}

	f := p.File()

	if detector != nil {
		defer func() {
			if pending := detector.Flush(); len(pending) > 0 {
				storage.Append(name, pending)
			}
		}()
	}

	buf := make([]byte, ReadBufferSize)
	for {
		select {
		case <-done:
			return
		default:
		}

		f.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := f.Read(buf)
		if n > 0 {
			data := buf[:n]
			if responder != nil {
				data = responder.Process(data)
			}
			if detector != nil {
				result := detector.Process(data)
				if result.Truncate {
					storage.Clear(name)
				}
				if len(result.DataAfter) > 0 {
					storage.Append(name, result.DataAfter)
				}
			} else {
				storage.Append(name, data)
			}
		}
		if err != nil && !isTimeout(err) {
			break
		}
	}

	cmd.Wait()
	p.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	h.pty = nil
	h.cmd = nil
	h.done = nil
	h.frameDetector = nil
	h.responder = nil

	h.state = StateStopped
	now := time.Now()
	h.stoppedAt = &now

	if meta, err := s.storage.LoadMeta(name); err == nil {
		meta.State = StateStopped
		meta.StoppedAt = &now
		s.storage.SaveMeta(name, meta)
	}
}

func isTimeout(err error) bool {
	if netErr, ok := err.(interface{ Timeout() bool }); ok {
		return netErr.Timeout()
	}
	return false
}

func (s *Server) handleList() Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]SessionInfo, 0, len(s.handles))
	for _, h := range s.handles {
		info := SessionInfo{
			Name:      h.name,
			PID:       h.pid,
			Command:   h.command,
			CreatedAt: h.createdAt.Format(time.RFC3339),
			State:     string(h.state),
		}
		if h.stoppedAt != nil {
			info.StoppedAt = h.stoppedAt.Format(time.RFC3339)
		}
		result = append(result, info)
	}

	return Response{Success: true, Data: result}
}

func (s *Server) handleRead(req Request) Response {
	if req.Snapshot {
		return s.handleSnapshot(req)
	}

	s.mu.Lock()
	h, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	sessState := h.state
	storage := s.storage
	s.mu.Unlock()

	meta, err := storage.LoadMeta(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("load meta: %v", err)}
	}

	mode := req.Mode
	if mode == "" {
		mode = ReadModeNew
	}

	var result string
	var totalLen int64

	switch mode {
	case ReadModeNew:
		totalLen, err = storage.Size(req.Name)
		if err != nil {
			return Response{Success: false, Error: fmt.Sprintf("get size: %v", err)}
		}

		readPos := meta.ReadPos
		if req.Cursor != "" {
			if meta.Cursors == nil {
				readPos = 0
			} else {
				readPos = meta.Cursors[req.Cursor]
			}
		}

		if readPos >= totalLen {
			result = ""
		} else {
			output, err := storage.ReadFrom(req.Name, readPos)
			if err != nil {
				return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
			}
			result = string(output)
		}

		if req.Cursor != "" {
			if meta.Cursors == nil {
				meta.Cursors = make(map[string]int64)
			}
			meta.Cursors[req.Cursor] = totalLen
		} else {
			meta.ReadPos = totalLen
		}
		storage.SaveMeta(req.Name, meta)
	default:
		output, err := storage.ReadAll(req.Name)
		if err != nil {
			return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
		}
		result = string(output)
		totalLen = int64(len(output))
	}

	if req.HeadLines > 0 || req.TailLines > 0 {
		result = LimitLines(result, req.HeadLines, req.TailLines)
	}

	return Response{Success: true, Data: map[string]interface{}{
		"output":   result,
		"position": totalLen,
		"state":    sessState,
	}}
}

func LimitLines(output string, head, tail int) string {
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")

	if head > 0 {
		if head >= len(lines) {
			return output
		}
		return strings.Join(lines[:head], "\n")
	}

	if tail > 0 {
		if tail >= len(lines) {
			return output
		}
		return strings.Join(lines[len(lines)-tail:], "\n")
	}

	return output
}

func (s *Server) handleSnapshot(req Request) Response {
	s.mu.Lock()
	h, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	if h.state != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is not running (snapshot requires a running TUI session)", req.Name)}
	}
	if h.frameDetector == nil {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is not in TUI mode (snapshot requires --tui)", req.Name)}
	}
	if h.pty == nil {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q PTY not available", req.Name)}
	}
	ptmx := h.pty.File()
	cmd := h.cmd
	storage := s.storage
	s.mu.Unlock()

	meta, err := storage.LoadMeta(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("load meta: %v", err)}
	}

	if sz, _ := storage.Size(req.Name); sz == 0 {
		coldDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(coldDeadline) {
			time.Sleep(SnapshotPollInterval)
			if sz, _ := storage.Size(req.Name); sz > 0 {
				break
			}
		}
	}

	storage.Clear(req.Name)
	s.mu.Lock()
	if h.frameDetector != nil {
		h.frameDetector.Reset()
		h.frameDetector.SetSnapshotMode(true)
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if h.frameDetector != nil {
			h.frameDetector.SetSnapshotMode(false)
		}
		s.mu.Unlock()
	}()

	tempCols := clampUint16(meta.Cols + 1)
	tempRows := clampUint16(meta.Rows + 1)
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: tempCols, Rows: tempRows}); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("temporary resize for snapshot: %v", err)}
	}
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGWINCH)
	}
	time.Sleep(SnapshotResizePause)

	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: clampUint16(meta.Cols), Rows: clampUint16(meta.Rows)}); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("resize for snapshot: %v", err)}
	}
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGWINCH)
	}

	settleMs := req.SettleMs
	if settleMs <= 0 {
		settleMs = DefaultSnapshotSettleMs
	}
	settleDuration := time.Duration(settleMs) * time.Millisecond

	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	maxTimeout := ClientDeadline - 5*time.Second
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	deadline := time.Now().Add(timeout)

	if err := waitForSettle(storage, req.Name, settleDuration, deadline); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("poll size: %v", err)}
	}

	output, err := storage.ReadAll(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
	}

	if len(output) == 0 && time.Now().Before(deadline) {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGWINCH)
		}

		waitForSettle(storage, req.Name, settleDuration*2, deadline)

		output, err = storage.ReadAll(req.Name)
		if err != nil {
			return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
		}
	}

	result := string(output)
	if req.HeadLines > 0 || req.TailLines > 0 {
		result = LimitLines(result, req.HeadLines, req.TailLines)
	}

	totalLen := int64(len(output))
	meta.ReadPos = totalLen
	storage.SaveMeta(req.Name, meta)

	return Response{Success: true, Data: map[string]interface{}{
		"output":   result,
		"position": totalLen,
		"state":    h.state,
	}}
}

func (s *Server) handleSend(req Request) Response {
	s.mu.Lock()
	h, ok := s.handles[req.Name]
	if !ok {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	if h.state != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is stopped", req.Name)}
	}
	p := h.pty
	s.mu.Unlock()

	if p == nil {
		return Response{Success: false, Error: fmt.Sprintf("session %q not running", req.Name)}
	}

	data := req.Input
	if req.Newline {
		data += "\n"
	}

	if _, err := p.File().WriteString(data); err != nil {
		return Response{Success: false, Error: err.Error()}
	}

	return Response{Success: true}
}

func (s *Server) handleStop(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, exists := s.handles[req.Name]
	if !exists {
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	if h.state == StateStopped {
		return Response{Success: true, Data: "already stopped"}
	}

	if h.done != nil {
		close(h.done)
		h.done = nil
	}

	if h.pty != nil {
		h.pty.Close()
		h.pty = nil
	}

	if h.cmd != nil {
		proc := h.cmd.Process
		h.cmd = nil
		proc.Signal(syscall.SIGTERM)
		go func() {
			time.Sleep(KillGracePeriod)
			proc.Signal(syscall.SIGKILL)
		}()
	}
	h.frameDetector = nil
	h.responder = nil

	h.state = StateStopped
	now := time.Now()
	h.stoppedAt = &now

	if meta, err := s.storage.LoadMeta(req.Name); err == nil {
		meta.State = StateStopped
		meta.StoppedAt = &now
		s.storage.SaveMeta(req.Name, meta)
	}

	return Response{Success: true}
}

func (s *Server) handleKill(req Request) Response {
	s.mu.Lock()

	h, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	var proc *os.Process
	if h.state == StateRunning {
		if h.done != nil {
			close(h.done)
		}
		if h.pty != nil {
			h.pty.Close()
		}
		if h.cmd != nil {
			proc = h.cmd.Process
		}
	}

	s.storage.Delete(req.Name)
	delete(s.handles, req.Name)
	s.mu.Unlock()

	if proc != nil {
		go func() {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(KillGracePeriod)
			proc.Signal(syscall.SIGKILL)
		}()
	}

	return Response{Success: true}
}

func (s *Server) handleSize(req Request) Response {
	s.mu.Lock()
	_, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	storage := s.storage
	s.mu.Unlock()

	size, err := storage.Size(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("get size: %v", err)}
	}
	return Response{Success: true, Data: map[string]interface{}{"size": size}}
}

func (s *Server) handleSearch(req Request) Response {
	if req.Before < 0 || req.After < 0 {
		return Response{Success: false, Error: "before and after must be non-negative"}
	}

	s.mu.Lock()
	_, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	storage := s.storage
	s.mu.Unlock()

	outputBytes, err := storage.ReadAll(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
	}

	output := string(outputBytes)
	if req.StripANSI {
		output = ansi.Strip(output)
	}

	patternStr := req.Pattern
	if req.IgnoreCase {
		patternStr = "(?i)" + patternStr
	}

	re, err := regexp.Compile(patternStr)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("invalid pattern: %v", err)}
	}

	lines := strings.Split(output, "\n")
	var matches []map[string]interface{}

	for i, line := range lines {
		if re.MatchString(line) {
			beforeStart := max(0, i-req.Before)
			afterEnd := min(len(lines), i+req.After+1)

			beforeLines := make([]string, 0, i-beforeStart)
			for j := beforeStart; j < i; j++ {
				beforeLines = append(beforeLines, lines[j])
			}

			afterLines := make([]string, 0, afterEnd-i-1)
			for j := i + 1; j < afterEnd; j++ {
				afterLines = append(afterLines, lines[j])
			}

			matches = append(matches, map[string]interface{}{
				"line_number": i + 1,
				"line":        line,
				"before":      beforeLines,
				"after":       afterLines,
			})
		}
	}

	return Response{Success: true, Data: map[string]interface{}{
		"matches":       matches,
		"total_matches": len(matches),
	}}
}

func (s *Server) handleInfo(req Request) Response {
	s.mu.Lock()
	h, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	storage := s.storage
	s.mu.Unlock()

	meta, err := storage.LoadMeta(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("load meta: %v", err)}
	}

	size, err := storage.Size(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("get size: %v", err)}
	}

	result := map[string]interface{}{
		"name":           h.name,
		"state":          string(h.state),
		"pid":            h.pid,
		"command":        h.command,
		"created_at":     h.createdAt.Format(time.RFC3339),
		"bytes_buffered": size,
		"read_position":  meta.ReadPos,
		"cols":           meta.Cols,
		"rows":           meta.Rows,
		"tui_mode":       meta.TUIMode,
	}

	if h.stoppedAt != nil {
		result["stopped_at"] = h.stoppedAt.Format(time.RFC3339)
	}

	if h.state == StateRunning {
		result["uptime_seconds"] = time.Since(h.createdAt).Seconds()
	}

	return Response{Success: true, Data: result}
}

func (s *Server) handleClear(req Request) Response {
	s.mu.Lock()
	_, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	storage := s.storage
	s.mu.Unlock()

	if err := storage.Clear(req.Name); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("clear: %v", err)}
	}

	return Response{Success: true}
}

func (s *Server) handleResize(req Request) Response {
	if req.Cols <= 0 && req.Rows <= 0 {
		return Response{Success: false, Error: "at least one of cols or rows is required"}
	}

	s.mu.Lock()
	h, exists := s.handles[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	if h.state != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is stopped", req.Name)}
	}

	p := h.pty
	if p == nil {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not running", req.Name)}
	}
	storage := s.storage
	s.mu.Unlock()

	meta, err := storage.LoadMeta(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("load meta: %v", err)}
	}

	cols := req.Cols
	rows := req.Rows
	if cols <= 0 {
		cols = meta.Cols
	}
	if rows <= 0 {
		rows = meta.Rows
	}

	if err := pty.Setsize(p.File(), &pty.Winsize{Cols: clampUint16(cols), Rows: clampUint16(rows)}); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("resize: %v", err)}
	}

	s.mu.Lock()
	if h.cmd != nil && h.cmd.Process != nil {
		h.cmd.Process.Signal(syscall.SIGWINCH)
	}
	s.mu.Unlock()

	meta.Cols = cols
	meta.Rows = rows
	if err := storage.SaveMeta(req.Name, meta); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("save meta: %v", err)}
	}

	return Response{Success: true, Data: map[string]interface{}{
		"cols": cols,
		"rows": rows,
	}}
}

func waitForSettle(storage OutputStorage, name string, settle time.Duration, deadline time.Time) error {
	lastChangeTime := time.Now()
	lastSize := int64(-1)
	for time.Now().Before(deadline) {
		time.Sleep(SnapshotPollInterval)
		size, err := storage.Size(name)
		if err != nil {
			return err
		}
		if size != lastSize {
			lastSize = size
			lastChangeTime = time.Now()
			continue
		}
		if size > 0 && time.Since(lastChangeTime) >= settle {
			return nil
		}
	}
	return nil
}

func clampUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}
