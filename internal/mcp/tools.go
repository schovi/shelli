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

type toolEntry struct {
	def     ToolDef
	handler func(json.RawMessage) (*CallToolResult, error)
}

type ToolRegistry struct {
	client  *daemon.Client
	entries []toolEntry
}

func (r *ToolRegistry) register(name, description string, schema map[string]interface{}, handler func(json.RawMessage) (*CallToolResult, error)) {
	r.entries = append(r.entries, toolEntry{
		def:     ToolDef{Name: name, Description: description, InputSchema: schema},
		handler: handler,
	})
}

var createSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Unique session name (alphanumeric start, may contain letters, numbers, dots, dashes, underscores; max 64 chars)",
			"pattern":     "^[A-Za-z0-9][A-Za-z0-9._-]*$",
		},
		"command": map[string]interface{}{
			"type":        "string",
			"description": "Command to run (e.g., 'python3', 'ssh user@host', 'psql -d mydb'). Defaults to user's shell.",
		},
		"env": map[string]interface{}{
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
			"description": "Environment variables to set (KEY=VALUE format)",
		},
		"cwd": map[string]interface{}{
			"type":        "string",
			"description": "Working directory for the session",
		},
		"cols": map[string]interface{}{
			"type":        "integer",
			"description": "Terminal columns (default: 80)",
		},
		"rows": map[string]interface{}{
			"type":        "integer",
			"description": "Terminal rows (default: 24)",
		},
		"tui": map[string]interface{}{
			"type":        "boolean",
			"description": "Enable TUI mode for apps like vim, htop. Auto-truncates buffer on frame boundaries to reduce storage.",
		},
		"if_not_exists": map[string]interface{}{
			"type":        "boolean",
			"description": "If true, return existing running session instead of error when session already exists.",
		},
	},
	"required": []string{"name"},
}

var execSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name",
		},
		"input": map[string]interface{}{
			"type":        "string",
			"description": "Command to send (newline added automatically, sent as literal text)",
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
	"required": []string{"name", "input"},
}

var sendSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name",
		},
		"input": map[string]interface{}{
			"type":        "string",
			"description": "Input to send (escape sequences interpreted). Mutually exclusive with inputs and input_base64.",
		},
		"inputs": map[string]interface{}{
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
			"description": "PREFERRED for sequences. Send multiple inputs in one call - each as separate PTY write. Use when sending message + Enter (e.g., [\"text\", \"\\r\"]), commands + confirmations, or any multi-step input. More efficient than multiple send calls. Mutually exclusive with input and input_base64.",
		},
		"input_base64": map[string]interface{}{
			"type":        "string",
			"description": "Input as base64 (for binary data). Sent as single write, no escape interpretation. Mutually exclusive with input and inputs.",
		},
	},
	"required": []string{"name"},
}

var readSchema = map[string]interface{}{
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
		"snapshot": map[string]interface{}{
			"type":        "boolean",
			"description": "Force TUI redraw via resize and read clean frame. Requires TUI mode (--tui on create). Incompatible with all, wait_pattern.",
		},
		"cursor": map[string]interface{}{
			"type":        "string",
			"description": "Named cursor for per-consumer read tracking. Each cursor maintains its own position.",
		},
	},
	"required": []string{"name"},
}

var listSchema = map[string]interface{}{
	"type":       "object",
	"properties": map[string]interface{}{},
}

var stopSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name to stop",
		},
	},
	"required": []string{"name"},
}

var killSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name to kill",
		},
	},
	"required": []string{"name"},
}

var infoSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name",
		},
	},
	"required": []string{"name"},
}

var clearSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name",
		},
	},
	"required": []string{"name"},
}

var resizeSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"name": map[string]interface{}{
			"type":        "string",
			"description": "Session name",
		},
		"cols": map[string]interface{}{
			"type":        "integer",
			"description": "Terminal columns (optional, keeps current if not specified)",
		},
		"rows": map[string]interface{}{
			"type":        "integer",
			"description": "Terminal rows (optional, keeps current if not specified)",
		},
	},
	"required": []string{"name"},
}

var searchSchema = map[string]interface{}{
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
}

func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{client: daemon.NewClient()}
	r.register("create", "Create a new interactive shell session. Use for REPLs, SSH, database CLIs, or any stateful workflow.", createSchema, r.callCreate)
	r.register("exec", "Send a command to a session and wait for output. Adds newline automatically, waits for output to settle or pattern match. Input is sent as literal text (no escape interpretation). For TUI apps or precise control, use 'send' with separate arguments: send session \"hello\" \"\\r\"", execSchema, r.callExec)
	r.register("send", "Send raw input to a session without waiting. Low-level command for precise control. Escape sequences (\\n, \\r, \\x03, etc.) are always interpreted. No newline added automatically.", sendSchema, r.callSend)
	r.register("read", "Read output from a session. Can read new output, all output, or wait for specific patterns.", readSchema, r.callRead)
	r.register("list", "List all active sessions with their status", listSchema, func(_ json.RawMessage) (*CallToolResult, error) {
		return r.callList()
	})
	r.register("stop", "Stop a running session but keep output accessible. Use this to preserve session output after process ends.", stopSchema, r.callStop)
	r.register("kill", "Kill/terminate a session and delete all output. Use 'stop' instead if you want to preserve output.", killSchema, r.callKill)
	r.register("info", "Get detailed information about a session including state, PID, command, buffer size, terminal dimensions, and uptime", infoSchema, r.callInfo)
	r.register("clear", "Clear the output buffer of a session and reset the read position. The session continues running.", clearSchema, r.callClear)
	r.register("resize", "Resize terminal dimensions of a running session. At least one of cols or rows must be specified.", resizeSchema, r.callResize)
	r.register("search", "Search session output buffer for regex patterns with context lines", searchSchema, r.callSearch)
	return r
}

func (r *ToolRegistry) List() []ToolDef {
	defs := make([]ToolDef, len(r.entries))
	for i, e := range r.entries {
		defs[i] = e.def
	}
	return defs
}

func (r *ToolRegistry) Call(name string, args json.RawMessage) (*CallToolResult, error) {
	if err := r.client.EnsureDaemon(); err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}
	for _, e := range r.entries {
		if e.def.Name == name {
			return e.handler(args)
		}
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

type CreateArgs struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Env         []string `json:"env"`
	Cwd         string   `json:"cwd"`
	Cols        int      `json:"cols"`
	Rows        int      `json:"rows"`
	TUI         bool     `json:"tui"`
	IfNotExists bool     `json:"if_not_exists"`
}

func (r *ToolRegistry) callCreate(args json.RawMessage) (*CallToolResult, error) {
	var a CreateArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	data, err := r.client.Create(a.Name, daemon.CreateOptions{
		Command:     a.Command,
		Env:         a.Env,
		Cwd:         a.Cwd,
		Cols:        a.Cols,
		Rows:        a.Rows,
		TUIMode:     a.TUI,
		IfNotExists: a.IfNotExists,
	})
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
	SettleMs    *int   `json:"settle_ms"`
	WaitPattern string `json:"wait_pattern"`
	TimeoutSec  int    `json:"timeout_sec"`
	StripAnsi   bool   `json:"strip_ansi"`
}

func (r *ToolRegistry) callExec(args json.RawMessage) (*CallToolResult, error) {
	var a ExecArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if a.WaitPattern != "" && a.SettleMs != nil && *a.SettleMs > 0 {
		return nil, fmt.Errorf("wait_pattern and settle_ms are mutually exclusive")
	}

	if a.Input == "" {
		return nil, fmt.Errorf("input is required")
	}

	settleMs := 0
	if a.SettleMs != nil {
		settleMs = *a.SettleMs
	}

	result, err := r.client.Exec(a.Name, daemon.ExecOptions{
		Input:       a.Input,
		SettleMs:    settleMs,
		WaitPattern: a.WaitPattern,
		TimeoutSec:  a.TimeoutSec,
		SettleSet:   a.SettleMs != nil,
	})
	if err != nil {
		if result == nil || result.Output == "" {
			return nil, err
		}
		output := result.Output
		if a.StripAnsi {
			output = ansi.Strip(output)
		}
		data, _ := json.MarshalIndent(map[string]interface{}{
			"input":    result.Input,
			"output":   output,
			"position": result.Position,
			"warning":  err.Error(),
		}, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
			IsError: true,
		}, nil
	}

	output := result.Output
	if a.StripAnsi {
		output = ansi.Strip(output)
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"input":    result.Input,
		"output":   output,
		"position": result.Position,
	}, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

type SendArgs struct {
	Name        string   `json:"name"`
	Input       string   `json:"input"`
	Inputs      []string `json:"inputs"`
	InputBase64 string   `json:"input_base64"`
}

func (r *ToolRegistry) callSend(args json.RawMessage) (*CallToolResult, error) {
	var a SendArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	// Validate mutually exclusive input options
	inputCount := 0
	if a.Input != "" {
		inputCount++
	}
	if len(a.Inputs) > 0 {
		inputCount++
	}
	if a.InputBase64 != "" {
		inputCount++
	}
	if inputCount == 0 {
		return nil, fmt.Errorf("one of input, inputs, or input_base64 is required")
	}
	if inputCount > 1 {
		return nil, fmt.Errorf("input, inputs, and input_base64 are mutually exclusive")
	}

	// Handle base64 input (no escape interpretation, single write)
	if a.InputBase64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(a.InputBase64)
		if err != nil {
			return nil, fmt.Errorf("decode input_base64: %w", err)
		}
		if err := r.client.Send(a.Name, string(decoded), false); err != nil {
			return nil, err
		}
		result := map[string]interface{}{
			"status": "sent",
			"count":  1,
			"bytes":  len(decoded),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
		}, nil
	}

	// Build list of inputs to send
	var inputs []string
	if len(a.Inputs) > 0 {
		inputs = a.Inputs
	} else {
		inputs = []string{a.Input}
	}

	// Send each input as separate write, with escape interpretation
	totalBytes := 0
	for _, input := range inputs {
		processed, err := escape.Interpret(input)
		if err != nil {
			return nil, fmt.Errorf("interpret escape sequences: %w", err)
		}

		if err := r.client.Send(a.Name, processed, false); err != nil {
			return nil, err
		}
		totalBytes += len(processed)
	}

	result := map[string]interface{}{
		"status": "sent",
		"count":  len(inputs),
		"bytes":  totalBytes,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
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
	Snapshot    bool   `json:"snapshot"`
	Cursor      string `json:"cursor"`
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

	if a.WaitPattern != "" && a.SettleMs > 0 {
		return nil, fmt.Errorf("wait_pattern and settle_ms are mutually exclusive")
	}

	if a.All && (a.WaitPattern != "" || a.SettleMs > 0) {
		return nil, fmt.Errorf("all cannot be combined with wait_pattern or settle_ms")
	}

	if a.Cursor != "" && a.Snapshot {
		return nil, fmt.Errorf("cursor and snapshot are mutually exclusive")
	}

	if a.Snapshot {
		if a.All {
			return nil, fmt.Errorf("snapshot and all are mutually exclusive")
		}
		if a.WaitPattern != "" {
			return nil, fmt.Errorf("snapshot and wait_pattern are mutually exclusive")
		}

		output, pos, err := r.client.Snapshot(a.Name, a.SettleMs, a.TimeoutSec, a.Head, a.Tail)
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

	mode := daemon.ReadModeNew
	if a.All || a.Head > 0 || a.Tail > 0 {
		mode = daemon.ReadModeAll
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
			func() (string, int, error) { return r.client.Read(a.Name, "all", 0, 0) },
			wait.Config{
				Pattern:       a.WaitPattern,
				SettleMs:      a.SettleMs,
				TimeoutSec:    timeoutSec,
				StartPosition: startPos,
				SizeFunc:      func() (int, error) { return r.client.Size(a.Name) },
			},
		)

		var warning string
		if err != nil {
			if output == "" {
				return nil, err
			}
			warning = err.Error()
		}

		if warning == "" {
			if a.Cursor != "" {
				r.client.ReadWithCursor(a.Name, "new", a.Cursor, 0, 0)
			} else {
				r.client.Read(a.Name, "new", 0, 0)
			}
		}

		if a.Head > 0 || a.Tail > 0 {
			output = daemon.LimitLines(output, a.Head, a.Tail)
		}

		if a.StripAnsi {
			output = ansi.Strip(output)
		}

		result := map[string]interface{}{
			"output":   output,
			"position": pos,
		}
		if warning != "" {
			result["warning"] = warning
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return &CallToolResult{
			Content: []ContentBlock{{Type: "text", Text: string(data)}},
			IsError: warning != "",
		}, nil
	}

	var output string
	var pos int
	var err error
	if a.Cursor != "" {
		output, pos, err = r.client.ReadWithCursor(a.Name, mode, a.Cursor, a.Head, a.Tail)
	} else {
		output, pos, err = r.client.Read(a.Name, mode, a.Head, a.Tail)
	}
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

type InfoArgs struct {
	Name string `json:"name"`
}

func (r *ToolRegistry) callInfo(args json.RawMessage) (*CallToolResult, error) {
	var a InfoArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	info, err := r.client.Info(a.Name)
	if err != nil {
		return nil, err
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
	}, nil
}

type ClearArgs struct {
	Name string `json:"name"`
}

func (r *ToolRegistry) callClear(args json.RawMessage) (*CallToolResult, error) {
	var a ClearArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if err := r.client.Clear(a.Name); err != nil {
		return nil, err
	}

	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("session %q cleared", a.Name)}},
	}, nil
}

type ResizeArgs struct {
	Name string `json:"name"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (r *ToolRegistry) callResize(args json.RawMessage) (*CallToolResult, error) {
	var a ResizeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}

	if a.Cols <= 0 && a.Rows <= 0 {
		return nil, fmt.Errorf("at least one of cols or rows is required")
	}

	if err := r.client.Resize(a.Name, a.Cols, a.Rows); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"name":   a.Name,
		"status": "resized",
	}
	if a.Cols > 0 {
		result["cols"] = a.Cols
	}
	if a.Rows > 0 {
		result["rows"] = a.Rows
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []ContentBlock{{Type: "text", Text: string(data)}},
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

	if before < 0 || after < 0 {
		return nil, fmt.Errorf("before, after, and around must be non-negative")
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
