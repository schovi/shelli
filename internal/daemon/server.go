package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

type Session struct {
	Name      string       `json:"name"`
	PID       int          `json:"pid"`
	Command   string       `json:"command"`
	State     SessionState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	StoppedAt *time.Time   `json:"stopped_at,omitempty"`
}

type SessionInfo struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
	State     string `json:"state"`
	StoppedAt string `json:"stopped_at,omitempty"`
}

type Server struct {
	mu             sync.Mutex
	sessions       map[string]*Session
	ptys           map[string]*ptyHandle
	cmds           map[string]*exec.Cmd
	doneChans      map[string]chan struct{}
	frameDetectors map[string]*ansi.FrameDetector
	responders     map[string]*ansi.TerminalResponder
	socketDir      string
	storage        OutputStorage
	listener       net.Listener

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
		sessions:        make(map[string]*Session),
		ptys:            make(map[string]*ptyHandle),
		cmds:            make(map[string]*exec.Cmd),
		doneChans:       make(map[string]chan struct{}),
		frameDetectors:  make(map[string]*ansi.FrameDetector),
		responders:      make(map[string]*ansi.TerminalResponder),
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

		s.sessions[name] = &Session{
			Name:      meta.Name,
			PID:       meta.PID,
			Command:   meta.Command,
			State:     meta.State,
			CreatedAt: meta.CreatedAt,
			StoppedAt: meta.StoppedAt,
		}
	}

	return nil
}

func SocketPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".shelli", "shelli.sock")
}

func (s *Server) Start() error {
	sockPath := SocketPath()
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
	for name, sess := range s.sessions {
		if sess.State == StateStopped && sess.StoppedAt != nil {
			if now.Sub(*sess.StoppedAt) > s.stoppedTTL {
				s.storage.Delete(name)
				delete(s.sessions, name)
			}
		}
	}
}

func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.cleanupStopChan)

	for name, sess := range s.sessions {
		if sess.State == StateRunning {
			if done, ok := s.doneChans[name]; ok {
				close(done)
			}
			if handle, ok := s.ptys[name]; ok {
				handle.Close()
			}
			if cmd, ok := s.cmds[name]; ok {
				cmd.Process.Kill()
				cmd.Wait()
			}
		}
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	os.Remove(SocketPath())
}

type Request struct {
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

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		s.sendResponse(conn, Response{Success: false, Error: err.Error()})
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

	if _, exists := s.sessions[req.Name]; exists {
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

	sess := &Session{
		Name:      req.Name,
		PID:       cmd.Process.Pid,
		Command:   command,
		State:     StateRunning,
		CreatedAt: now,
	}

	handle := &ptyHandle{f: ptmx}
	s.sessions[req.Name] = sess
	s.ptys[req.Name] = handle
	s.cmds[req.Name] = cmd
	s.doneChans[req.Name] = make(chan struct{})
	if req.TUIMode {
		s.frameDetectors[req.Name] = ansi.NewFrameDetector(ansi.DefaultTUIStrategy())
		s.responders[req.Name] = ansi.NewTerminalResponder(ptmx)
	}

	go s.captureOutput(req.Name, handle, cmd)

	return Response{Success: true, Data: map[string]interface{}{
		"name":       sess.Name,
		"pid":        sess.PID,
		"command":    sess.Command,
		"created_at": sess.CreatedAt,
		"cols":       cols,
		"rows":       rows,
	}}
}

func (s *Server) captureOutput(name string, handle *ptyHandle, cmd *exec.Cmd) {
	s.mu.Lock()
	done := s.doneChans[name]
	detector := s.frameDetectors[name]
	responder := s.responders[name]
	storage := s.storage
	s.mu.Unlock()

	f := handle.File()

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
	handle.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.ptys, name)
	delete(s.cmds, name)
	delete(s.doneChans, name)
	delete(s.frameDetectors, name)
	delete(s.responders, name)

	if sess, ok := s.sessions[name]; ok {
		sess.State = StateStopped
		now := time.Now()
		sess.StoppedAt = &now

		if meta, err := s.storage.LoadMeta(name); err == nil {
			meta.State = StateStopped
			meta.StoppedAt = &now
			s.storage.SaveMeta(name, meta)
		}
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

	result := make([]SessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		info := SessionInfo{
			Name:      sess.Name,
			PID:       sess.PID,
			Command:   sess.Command,
			CreatedAt: sess.CreatedAt.Format(time.RFC3339),
			State:     string(sess.State),
		}
		if sess.StoppedAt != nil {
			info.StoppedAt = sess.StoppedAt.Format(time.RFC3339)
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
	sess, exists := s.sessions[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	sessState := sess.State
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
	sess, exists := s.sessions[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	if sess.State != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is not running (snapshot requires a running TUI session)", req.Name)}
	}
	_, hasFD := s.frameDetectors[req.Name]
	if !hasFD {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is not in TUI mode (snapshot requires --tui)", req.Name)}
	}
	handle, ok := s.ptys[req.Name]
	if !ok {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q PTY not available", req.Name)}
	}
	ptmx := handle.File()
	cmd := s.cmds[req.Name]
	storage := s.storage
	s.mu.Unlock()

	meta, err := storage.LoadMeta(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("load meta: %v", err)}
	}

	// Cold start: if storage is empty, wait up to 2s for the app to produce initial content.
	// This handles the case where snapshot is called before the app has rendered anything.
	if sz, _ := storage.Size(req.Name); sz == 0 {
		coldDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(coldDeadline) {
			time.Sleep(SnapshotPollInterval)
			if sz, _ := storage.Size(req.Name); sz > 0 {
				break
			}
		}
	}

	// Clear storage and reset frame detector before resize cycle.
	// This ensures the settle loop starts from size=0 and waits for fresh data,
	// preventing races where captureOutput's Clear+Append can be seen as empty.
	storage.Clear(req.Name)
	s.mu.Lock()
	if fd, ok := s.frameDetectors[req.Name]; ok {
		fd.Reset()
		fd.SetSnapshotMode(true)
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if fd, ok := s.frameDetectors[req.Name]; ok {
			fd.SetSnapshotMode(false)
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
	lastChangeTime := time.Now()
	lastSize := int64(-1)

	for time.Now().Before(deadline) {
		time.Sleep(SnapshotPollInterval)

		size, err := storage.Size(req.Name)
		if err != nil {
			return Response{Success: false, Error: fmt.Sprintf("poll size: %v", err)}
		}

		if size != lastSize {
			lastSize = size
			lastChangeTime = time.Now()
			continue
		}

		if size > 0 && time.Since(lastChangeTime) >= settleDuration {
			break
		}
	}

	output, err := storage.ReadAll(req.Name)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("read output: %v", err)}
	}

	// Retry once if empty and time remains: some apps need a second SIGWINCH nudge
	if len(output) == 0 && time.Now().Before(deadline) {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGWINCH)
		}

		retrySettle := settleDuration * 2
		lastChangeTime = time.Now()
		lastSize = int64(-1)

		for time.Now().Before(deadline) {
			time.Sleep(SnapshotPollInterval)

			size, err := storage.Size(req.Name)
			if err != nil {
				break
			}

			if size != lastSize {
				lastSize = size
				lastChangeTime = time.Now()
				continue
			}

			if size > 0 && time.Since(lastChangeTime) >= retrySettle {
				break
			}
		}

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
		"state":    sess.State,
	}}
}

func (s *Server) handleSend(req Request) Response {
	s.mu.Lock()
	sess, ok := s.sessions[req.Name]
	if !ok {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}
	if sess.State != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is stopped", req.Name)}
	}
	handle, ok := s.ptys[req.Name]
	s.mu.Unlock()

	if !ok {
		return Response{Success: false, Error: fmt.Sprintf("session %q not running", req.Name)}
	}

	data := req.Input
	if req.Newline {
		data += "\n"
	}

	if _, err := handle.File().WriteString(data); err != nil {
		return Response{Success: false, Error: err.Error()}
	}

	return Response{Success: true}
}

func (s *Server) handleStop(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[req.Name]
	if !exists {
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	if sess.State == StateStopped {
		return Response{Success: true, Data: "already stopped"}
	}

	if done, ok := s.doneChans[req.Name]; ok {
		close(done)
		delete(s.doneChans, req.Name)
	}

	if handle, ok := s.ptys[req.Name]; ok {
		handle.Close()
		delete(s.ptys, req.Name)
	}

	if cmd, ok := s.cmds[req.Name]; ok {
		proc := cmd.Process
		proc.Signal(syscall.SIGTERM)
		go func() {
			time.Sleep(KillGracePeriod)
			proc.Signal(syscall.SIGKILL)
			cmd.Wait()
		}()
		delete(s.cmds, req.Name)
	}
	delete(s.frameDetectors, req.Name)
	delete(s.responders, req.Name)

	sess.State = StateStopped
	now := time.Now()
	sess.StoppedAt = &now

	if meta, err := s.storage.LoadMeta(req.Name); err == nil {
		meta.State = StateStopped
		meta.StoppedAt = &now
		s.storage.SaveMeta(req.Name, meta)
	}

	return Response{Success: true}
}

func (s *Server) handleKill(req Request) Response {
	s.mu.Lock()

	sess, exists := s.sessions[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	var proc *os.Process
	if sess.State == StateRunning {
		if done, ok := s.doneChans[req.Name]; ok {
			close(done)
			delete(s.doneChans, req.Name)
		}
		if handle, ok := s.ptys[req.Name]; ok {
			handle.Close()
			delete(s.ptys, req.Name)
		}
		if cmd, ok := s.cmds[req.Name]; ok {
			proc = cmd.Process
			delete(s.cmds, req.Name)
		}
	}

	s.storage.Delete(req.Name)
	delete(s.sessions, req.Name)
	delete(s.frameDetectors, req.Name)
	delete(s.responders, req.Name)
	s.mu.Unlock()

	if proc != nil {
		go func() {
			proc.Signal(syscall.SIGTERM)
			time.Sleep(KillGracePeriod)
			proc.Signal(syscall.SIGKILL)
			proc.Wait()
		}()
	}

	return Response{Success: true}
}

func (s *Server) handleSize(req Request) Response {
	s.mu.Lock()
	_, exists := s.sessions[req.Name]
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
	_, exists := s.sessions[req.Name]
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
	sess, exists := s.sessions[req.Name]
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
		"name":           sess.Name,
		"state":          string(sess.State),
		"pid":            sess.PID,
		"command":        sess.Command,
		"created_at":     sess.CreatedAt.Format(time.RFC3339),
		"bytes_buffered": size,
		"read_position":  meta.ReadPos,
		"cols":           meta.Cols,
		"rows":           meta.Rows,
		"tui_mode":       meta.TUIMode,
	}

	if sess.StoppedAt != nil {
		result["stopped_at"] = sess.StoppedAt.Format(time.RFC3339)
	}

	if sess.State == StateRunning {
		result["uptime_seconds"] = time.Since(sess.CreatedAt).Seconds()
	}

	return Response{Success: true, Data: result}
}

func (s *Server) handleClear(req Request) Response {
	s.mu.Lock()
	_, exists := s.sessions[req.Name]
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
	sess, exists := s.sessions[req.Name]
	if !exists {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	if sess.State != StateRunning {
		s.mu.Unlock()
		return Response{Success: false, Error: fmt.Sprintf("session %q is stopped", req.Name)}
	}

	handle, ok := s.ptys[req.Name]
	if !ok {
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

	if err := pty.Setsize(handle.File(), &pty.Winsize{Cols: clampUint16(cols), Rows: clampUint16(rows)}); err != nil {
		return Response{Success: false, Error: fmt.Sprintf("resize: %v", err)}
	}

	// Send SIGWINCH explicitly to ensure the process receives it
	// (pty.Setsize should trigger this via kernel, but explicit signal is more reliable)
	s.mu.Lock()
	if cmd, ok := s.cmds[req.Name]; ok && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGWINCH)
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

func clampUint16(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}
