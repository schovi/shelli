package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/schovi/shelli/internal/ansi"
	"github.com/schovi/shelli/internal/daemon"
	"github.com/schovi/shelli/internal/escape"
	"github.com/schovi/shelli/internal/wait"
)

type ToolRegistry struct {
	client *daemon.Client
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		client: daemon.NewClient(),
	}
}

func (r *ToolRegistry) List() []ToolDef {
	return []ToolDef{
		{
			Name:        "create",
			Description: "Create a new interactive shell session. Use for REPLs, SSH, database CLIs, or any stateful workflow.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Unique session name (e.g., 'python-repl', 'ssh-prod', 'postgres-db')",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command to run (e.g., 'python3', 'ssh user@host', 'psql -d mydb'). Defaults to user's shell.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "exec",
			Description: "Send input to a session and wait for output. Primary command for AI interaction. Sends with newline, waits for output to settle or pattern match. NOTE: For TUI apps with input buffers (like chat interfaces), use 'send' instead with two-step pattern.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name",
					},
					"input": map[string]interface{}{
						"type":        "string",
						"description": "Input to send (newline added automatically). Mutually exclusive with input_base64.",
					},
					"input_base64": map[string]interface{}{
						"type":        "string",
						"description": "Input as base64 (fallback when JSON escaping is too complex). Mutually exclusive with input.",
					},
					"settle_ms": map[string]interface{}{
						"type":        "integer",
						"description": "Wait for N ms of silence (default: 500). Mutually exclusive with wait_pattern.",
					},
					"wait_pattern": map[string]interface{}{
						"type":        "string",
						"description": "Wait for regex pattern match (e.g., '>>>' for Python prompt). Mutually exclusive with settle_ms.",
					},
					"timeout_sec": map[string]interface{}{
						"type":        "integer",
						"description": "Max wait time in seconds (default: 10)",
					},
					"strip_ansi": map[string]interface{}{
						"type":        "boolean",
						"description": "Remove ANSI escape codes from output (default: false)",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "send",
			Description: "Send input to a session without waiting. Use for control characters (Ctrl+C, Ctrl+D), answering prompts, or TUI apps that need two-step input (send message, then raw \\r to submit).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name",
					},
					"input": map[string]interface{}{
						"type":        "string",
						"description": "Input to send. Use escape sequences for control chars: \\x03 (Ctrl+C), \\x04 (Ctrl+D), \\t (Tab), \\r (submit in TUIs). Mutually exclusive with input_base64.",
					},
					"input_base64": map[string]interface{}{
						"type":        "string",
						"description": "Input as base64 (fallback when JSON escaping is too complex). Mutually exclusive with input.",
					},
					"raw": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, interprets escape sequences and does NOT add newline. If false (default), adds newline. TIP: For TUI chat apps, first send message (raw=false), then send \\r with raw=true to submit.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "read",
			Description: "Read output from a session. Can read new output, all output, or wait for specific patterns.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name",
					},
					"all": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, read all output from session start. If false (default), read only new output since last read. Mutually exclusive with head/tail.",
					},
					"head": map[string]interface{}{
						"type":        "integer",
						"description": "Return first N lines of buffer. Mutually exclusive with all/tail.",
					},
					"tail": map[string]interface{}{
						"type":        "integer",
						"description": "Return last N lines of buffer. Mutually exclusive with all/head.",
					},
					"wait_pattern": map[string]interface{}{
						"type":        "string",
						"description": "Wait for regex pattern match before returning",
					},
					"settle_ms": map[string]interface{}{
						"type":        "integer",
						"description": "Wait for N ms of silence before returning",
					},
					"timeout_sec": map[string]interface{}{
						"type":        "integer",
						"description": "Max wait time in seconds (default: 10, only used with wait_pattern or settle_ms)",
					},
					"strip_ansi": map[string]interface{}{
						"type":        "boolean",
						"description": "Remove ANSI escape codes from output",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "list",
			Description: "List all active sessions with their status",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "stop",
			Description: "Stop a running session but keep output accessible. Use this to preserve session output after process ends.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name to stop",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "kill",
			Description: "Kill/terminate a session and delete all output. Use 'stop' instead if you want to preserve output.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name to kill",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "search",
			Description: "Search session output buffer for regex patterns with context lines",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Session name",
					},
					"pattern": map[string]interface{}{
						"type":        "string",
						"description": "Regex pattern to search for",
					},
					"before": map[string]interface{}{
						"type":        "integer",
						"description": "Lines of context before each match (default: 0)",
					},
					"after": map[string]interface{}{
						"type":        "integer",
						"description": "Lines of context after each match (default: 0)",
					},
					"around": map[string]interface{}{
						"type":        "integer",
						"description": "Lines of context before AND after (shorthand for before+after). Mutually exclusive with before/after.",
					},
					"ignore_case": map[string]interface{}{
						"type":        "boolean",
						"description": "Case-insensitive search (default: false)",
					},
					"strip_ansi": map[string]interface{}{
						"type":        "boolean",
						"description": "Strip ANSI escape codes before searching (default: false)",
					},
				},
				"required": []string{"name", "pattern"},
			},
		},
	}
}

func (r *ToolRegistry) Call(name string, args json.RawMessage) (*CallToolResult, error) {
	if err := r.client.EnsureDaemon(); err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}

	switch name {
	case "create":
		return r.callCreate(args)
	case "exec":
		return r.callExec(args)
	case "send":
		return r.callSend(args)
	case "read":
		return r.callRead(args)
	case "list":
		return r.callList()
	case "stop":
		return r.callStop(args)
	case "kill":
		return r.callKill(args)
	case "search":
		return r.callSearch(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

type CreateArgs struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

func (r *ToolRegistry) callCreate(args json.RawMessage) (*CallToolResult, error) {
	var a CreateArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	data, err := r.client.Create(a.Name, a.Command)
	if err != nil {
		return nil, err
	}

	output, _ := json.MarshalIndent(data, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(output)}},
	}, nil
}

type ExecArgs struct {
	Name        string `json:"name"`
	Input       string `json:"input"`
	InputBase64 string `json:"input_base64"`
	SettleMs    int    `json:"settle_ms"`
	WaitPattern string `json:"wait_pattern"`
	TimeoutSec  int    `json:"timeout_sec"`
	StripAnsi   bool   `json:"strip_ansi"`
}

func (r *ToolRegistry) callExec(args json.RawMessage) (*CallToolResult, error) {
	var a ExecArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if a.WaitPattern != "" && a.SettleMs > 0 {
		return nil, fmt.Errorf("wait_pattern and settle_ms are mutually exclusive")
	}

	if a.Input == "" && a.InputBase64 == "" {
		return nil, fmt.Errorf("input or input_base64 is required")
	}

	input := a.Input
	if a.InputBase64 != "" {
		if a.Input != "" {
			return nil, fmt.Errorf("input and input_base64 are mutually exclusive")
		}
		decoded, err := base64.StdEncoding.DecodeString(a.InputBase64)
		if err != nil {
			return nil, fmt.Errorf("decode input_base64: %w", err)
		}
		input = string(decoded)
	}

	_, startPos, err := r.client.Read(a.Name, "all", 0, 0)
	if err != nil {
		return nil, err
	}

	if err := r.client.Send(a.Name, input, true); err != nil {
		return nil, err
	}

	settleMs := a.SettleMs
	if a.WaitPattern == "" && settleMs == 0 {
		settleMs = 500
	}

	timeoutSec := a.TimeoutSec
	if timeoutSec == 0 {
		timeoutSec = 10
	}

	output, pos, err := wait.ForOutput(
		func() (string, int, error) { return r.client.Read(a.Name, "all", 0, 0) },
		wait.Config{
			Pattern:       a.WaitPattern,
			SettleMs:      settleMs,
			TimeoutSec:    timeoutSec,
			StartPosition: startPos,
		},
	)
	if err != nil {
		return nil, err
	}

	if a.StripAnsi {
		output = ansi.Strip(output)
	}

	result := map[string]interface{}{
		"input":    input,
		"output":   output,
		"position": pos,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

type SendArgs struct {
	Name        string `json:"name"`
	Input       string `json:"input"`
	InputBase64 string `json:"input_base64"`
	Raw         bool   `json:"raw"`
}

func (r *ToolRegistry) callSend(args json.RawMessage) (*CallToolResult, error) {
	var a SendArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if a.Input == "" && a.InputBase64 == "" {
		return nil, fmt.Errorf("input or input_base64 is required")
	}

	input := a.Input
	if a.InputBase64 != "" {
		if a.Input != "" {
			return nil, fmt.Errorf("input and input_base64 are mutually exclusive")
		}
		decoded, err := base64.StdEncoding.DecodeString(a.InputBase64)
		if err != nil {
			return nil, fmt.Errorf("decode input_base64: %w", err)
		}
		input = string(decoded)
	}

	addNewline := true

	if a.Raw {
		var err error
		input, err = escape.Interpret(input)
		if err != nil {
			return nil, fmt.Errorf("interpret escape sequences: %w", err)
		}
		addNewline = false
	}

	if err := r.client.Send(a.Name, input, addNewline); err != nil {
		return nil, err
	}

	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: "sent"}},
	}, nil
}

type ReadArgs struct {
	Name        string `json:"name"`
	All         bool   `json:"all"`
	Head        int    `json:"head"`
	Tail        int    `json:"tail"`
	WaitPattern string `json:"wait_pattern"`
	SettleMs    int    `json:"settle_ms"`
	TimeoutSec  int    `json:"timeout_sec"`
	StripAnsi   bool   `json:"strip_ansi"`
}

func (r *ToolRegistry) callRead(args json.RawMessage) (*CallToolResult, error) {
	var a ReadArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	modeCount := 0
	if a.All {
		modeCount++
	}
	if a.Head > 0 {
		modeCount++
	}
	if a.Tail > 0 {
		modeCount++
	}
	if modeCount > 1 {
		return nil, fmt.Errorf("all, head, and tail are mutually exclusive")
	}

	if a.Head < 0 || a.Tail < 0 {
		return nil, fmt.Errorf("head and tail require positive integers")
	}

	mode := "new"
	if a.All || a.Head > 0 || a.Tail > 0 {
		mode = "all"
	}

	if a.WaitPattern != "" || a.SettleMs > 0 {
		_, startPos, err := r.client.Read(a.Name, "all", 0, 0)
		if err != nil {
			return nil, err
		}

		timeoutSec := a.TimeoutSec
		if timeoutSec == 0 {
			timeoutSec = 10
		}

		output, pos, err := wait.ForOutput(
			func() (string, int, error) { return r.client.Read(a.Name, "all", a.Head, a.Tail) },
			wait.Config{
				Pattern:       a.WaitPattern,
				SettleMs:      a.SettleMs,
				TimeoutSec:    timeoutSec,
				StartPosition: startPos,
			},
		)
		if err != nil {
			return nil, err
		}

		if a.StripAnsi {
			output = ansi.Strip(output)
		}

		result := map[string]interface{}{
			"output":   output,
			"position": pos,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil
	}

	output, pos, err := r.client.Read(a.Name, mode, a.Head, a.Tail)
	if err != nil {
		return nil, err
	}

	if a.StripAnsi {
		output = ansi.Strip(output)
	}

	result := map[string]interface{}{
		"output":   output,
		"position": pos,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

func (r *ToolRegistry) callList() (*CallToolResult, error) {
	sessions, err := r.client.List()
	if err != nil {
		return nil, err
	}

	data, _ := json.MarshalIndent(sessions, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

type StopArgs struct {
	Name string `json:"name"`
}

func (r *ToolRegistry) callStop(args json.RawMessage) (*CallToolResult, error) {
	var a StopArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if err := r.client.Stop(a.Name); err != nil {
		return nil, err
	}

	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("session %q stopped", a.Name)}},
	}, nil
}

type KillArgs struct {
	Name string `json:"name"`
}

func (r *ToolRegistry) callKill(args json.RawMessage) (*CallToolResult, error) {
	var a KillArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if err := r.client.Kill(a.Name); err != nil {
		return nil, err
	}

	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("session %q killed", a.Name)}},
	}, nil
}

type SearchArgs struct {
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`
	Before     int    `json:"before"`
	After      int    `json:"after"`
	Around     int    `json:"around"`
	IgnoreCase bool   `json:"ignore_case"`
	StripAnsi  bool   `json:"strip_ansi"`
}

func (r *ToolRegistry) callSearch(args json.RawMessage) (*CallToolResult, error) {
	var a SearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if a.Around > 0 && (a.Before > 0 || a.After > 0) {
		return nil, fmt.Errorf("around is mutually exclusive with before/after")
	}

	before := a.Before
	after := a.After
	if a.Around > 0 {
		before = a.Around
		after = a.Around
	}

	resp, err := r.client.Search(daemon.SearchRequest{
		Name:       a.Name,
		Pattern:    a.Pattern,
		Before:     before,
		After:      after,
		IgnoreCase: a.IgnoreCase,
		StripANSI:  a.StripAnsi,
	})
	if err != nil {
		return nil, err
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}
