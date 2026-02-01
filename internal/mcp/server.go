package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

const ProtocolVersion = "2024-11-05"

type Server struct {
	tools   *ToolRegistry
	version string
	reader  *bufio.Reader
	writer  io.Writer
	mu      sync.Mutex
}

func NewServer(tools *ToolRegistry, version string) *Server {
	return &Server{
		tools:   tools,
		version: version,
		reader:  bufio.NewReader(os.Stdin),
		writer:  os.Stdout,
	}
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type ToolsListResult struct {
	Tools []ToolDef `json:"tools"`
}

type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (s *Server) Run() error {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *Server) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// No response needed
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.sendResult(req.ID, map[string]string{})
	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *Server) handleInitialize(req *Request) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]any{
			"tools": map[string]any{},
		},
	}
	result.ServerInfo.Name = "shelli"
	result.ServerInfo.Version = s.version

	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	tools := s.tools.List()
	s.sendResult(req.ID, ToolsListResult{Tools: tools})
}

func (s *Server) handleToolsCall(req *Request) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	result, err := s.tools.Call(params.Name, params.Arguments)
	if err != nil {
		s.sendResult(req.ID, CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		})
		return
	}

	s.sendResult(req.ID, result)
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	s.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id interface{}, code int, message, data string) {
	s.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}

func (s *Server) send(resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, _ := json.Marshal(resp)
	s.writer.Write(data)
	s.writer.Write([]byte("\n"))
}
