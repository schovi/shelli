package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Session struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
	ReadPos   int       `json:"read_pos"`
}

type SessionInfo struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
	Running   bool   `json:"running"`
}

type Server struct {
	mu            sync.Mutex
	sessions      map[string]*Session
	ptys          map[string]*os.File
	cmds          map[string]*exec.Cmd
	outputs       map[string][]byte
	doneChans     map[string]chan struct{}
	dataDir       string
	listener      net.Listener
	maxOutputSize int
}

type ServerOption func(*Server)

func WithMaxOutputSize(size int) ServerOption {
	return func(s *Server) {
		s.maxOutputSize = size
	}
}

func NewServer(opts ...ServerOption) (*Server, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(homeDir, ".shelli")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	s := &Server{
		sessions:      make(map[string]*Session),
		ptys:          make(map[string]*os.File),
		cmds:          make(map[string]*exec.Cmd),
		outputs:       make(map[string][]byte),
		doneChans:     make(map[string]chan struct{}),
		dataDir:       dataDir,
		maxOutputSize: DefaultMaxOutputSize,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
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

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.listener == nil {
				return nil
			}
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name := range s.sessions {
		if done, ok := s.doneChans[name]; ok {
			close(done)
		}
		if ptmx, ok := s.ptys[name]; ok {
			ptmx.Close()
		}
		if cmd, ok := s.cmds[name]; ok {
			cmd.Process.Kill()
		}
	}

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	os.Remove(SocketPath())
}

type Request struct {
	Action  string `json:"action"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
	Input   string `json:"input,omitempty"`
	Newline bool   `json:"newline,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

type Response struct {
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Data     interface{} `json:"data,omitempty"`
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
	case "kill":
		resp = s.handleKill(req)
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
		cmd = exec.Command("sh", "-c", command)
	} else {
		cmd = exec.Command(command)
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return Response{Success: false, Error: fmt.Sprintf("start pty: %v", err)}
	}

	sess := &Session{
		Name:      req.Name,
		PID:       cmd.Process.Pid,
		Command:   command,
		CreatedAt: time.Now(),
		ReadPos:   0,
	}

	s.sessions[req.Name] = sess
	s.ptys[req.Name] = ptmx
	s.cmds[req.Name] = cmd
	s.outputs[req.Name] = []byte{}
	s.doneChans[req.Name] = make(chan struct{})

	go s.captureOutput(req.Name, ptmx, cmd)

	return Response{Success: true, Data: map[string]interface{}{
		"name":       sess.Name,
		"pid":        sess.PID,
		"command":    sess.Command,
		"created_at": sess.CreatedAt,
	}}
}

func (s *Server) captureOutput(name string, ptmx *os.File, cmd *exec.Cmd) {
	s.mu.Lock()
	done := s.doneChans[name]
	s.mu.Unlock()

	buf := make([]byte, ReadBufferSize)
	for {
		select {
		case <-done:
			return
		default:
		}

		ptmx.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.outputs[name] = append(s.outputs[name], buf[:n]...)
			if len(s.outputs[name]) > s.maxOutputSize {
				excess := len(s.outputs[name]) - s.maxOutputSize
				s.outputs[name] = s.outputs[name][excess:]
				if sess, ok := s.sessions[name]; ok && sess.ReadPos > 0 {
					sess.ReadPos = max(0, sess.ReadPos-excess)
				}
			}
			s.mu.Unlock()
		}
		if err != nil && !isTimeout(err) {
			break
		}
	}

	cmd.Wait()
	ptmx.Close()

	s.mu.Lock()
	delete(s.ptys, name)
	delete(s.cmds, name)
	delete(s.doneChans, name)
	s.mu.Unlock()
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
		running := s.isRunning(sess.PID)
		result = append(result, SessionInfo{
			Name:      sess.Name,
			PID:       sess.PID,
			Command:   sess.Command,
			CreatedAt: sess.CreatedAt.Format(time.RFC3339),
			Running:   running,
		})
	}

	return Response{Success: true, Data: result}
}

func (s *Server) handleRead(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[req.Name]
	if !exists {
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	output := s.outputs[req.Name]
	totalLen := len(output)

	mode := req.Mode
	if mode == "" {
		mode = "new"
	}

	var result string
	switch mode {
	case "new":
		if sess.ReadPos >= totalLen {
			result = ""
		} else {
			result = string(output[sess.ReadPos:])
			sess.ReadPos = totalLen
		}
	default:
		result = string(output)
	}

	return Response{Success: true, Data: map[string]interface{}{
		"output":   result,
		"position": totalLen,
	}}
}

func (s *Server) handleSend(req Request) Response {
	s.mu.Lock()
	ptmx, ok := s.ptys[req.Name]
	s.mu.Unlock()

	if !ok {
		return Response{Success: false, Error: fmt.Sprintf("session %q not running", req.Name)}
	}

	data := req.Input
	if req.Newline {
		data += "\n"
	}

	if _, err := ptmx.WriteString(data); err != nil {
		return Response{Success: false, Error: err.Error()}
	}

	return Response{Success: true}
}

func (s *Server) handleKill(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[req.Name]
	if !exists {
		return Response{Success: false, Error: fmt.Sprintf("session %q not found", req.Name)}
	}

	if done, ok := s.doneChans[req.Name]; ok {
		close(done)
		delete(s.doneChans, req.Name)
	}

	if ptmx, ok := s.ptys[req.Name]; ok {
		ptmx.Close()
		delete(s.ptys, req.Name)
	}

	if cmd, ok := s.cmds[req.Name]; ok {
		cmd.Process.Kill()
		delete(s.cmds, req.Name)
	}

	proc, err := os.FindProcess(sess.PID)
	if err == nil {
		proc.Signal(syscall.SIGTERM)
		time.Sleep(KillGracePeriod)
		proc.Signal(syscall.SIGKILL)
	}

	delete(s.sessions, req.Name)
	delete(s.outputs, req.Name)

	return Response{Success: true}
}

func (s *Server) isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
