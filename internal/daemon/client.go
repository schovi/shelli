package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) EnsureDaemon() error {
	if c.Ping() {
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	cmd := exec.Command(exePath, "daemon")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	deadline := time.Now().Add(DaemonStartTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(DaemonPollInterval)
		if c.Ping() {
			return nil
		}
	}

	return fmt.Errorf("daemon failed to start")
}

func (c *Client) Ping() bool {
	resp, err := c.send(Request{Action: "ping"})
	return err == nil && resp.Success
}

type CreateOptions struct {
	Command string
	Env     []string
	Cwd     string
	Cols    int
	Rows    int
	TUIMode bool
}

func (c *Client) Create(name string, opts CreateOptions) (map[string]interface{}, error) {
	if err := ValidateSessionName(name); err != nil {
		return nil, err
	}

	resp, err := c.send(Request{
		Action:  "create",
		Name:    name,
		Command: opts.Command,
		Env:     opts.Env,
		Cwd:     opts.Cwd,
		Cols:    opts.Cols,
		Rows:    opts.Rows,
		TUIMode: opts.TUIMode,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return extractMapData(resp)
}

func (c *Client) List() ([]SessionInfo, error) {
	resp, err := c.send(Request{Action: "list"})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal response: %w", err)
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("unmarshal sessions: %w", err)
	}
	return sessions, nil
}

func (c *Client) Read(name, mode string, headLines, tailLines int) (string, int, error) {
	resp, err := c.send(Request{
		Action:    "read",
		Name:      name,
		Mode:      mode,
		HeadLines: headLines,
		TailLines: tailLines,
	})
	if err != nil {
		return "", 0, err
	}
	if !resp.Success {
		return "", 0, fmt.Errorf("%s", resp.Error)
	}

	data, err := extractMapData(resp)
	if err != nil {
		return "", 0, err
	}

	output, ok := data["output"].(string)
	if !ok {
		return "", 0, fmt.Errorf("missing or invalid output field")
	}
	posFloat, ok := data["position"].(float64)
	if !ok {
		return "", 0, fmt.Errorf("missing or invalid position field")
	}
	return output, int(posFloat), nil
}

func (c *Client) Snapshot(name string, settleMs, timeoutSec, headLines, tailLines int) (string, int, error) {
	resp, err := c.send(Request{
		Action:     "read",
		Name:       name,
		Snapshot:   true,
		SettleMs:   settleMs,
		TimeoutSec: timeoutSec,
		HeadLines:  headLines,
		TailLines:  tailLines,
	})
	if err != nil {
		return "", 0, err
	}
	if !resp.Success {
		return "", 0, fmt.Errorf("%s", resp.Error)
	}

	data, err := extractMapData(resp)
	if err != nil {
		return "", 0, err
	}

	output, ok := data["output"].(string)
	if !ok {
		return "", 0, fmt.Errorf("missing or invalid output field")
	}
	posFloat, ok := data["position"].(float64)
	if !ok {
		return "", 0, fmt.Errorf("missing or invalid position field")
	}
	return output, int(posFloat), nil
}

func (c *Client) Send(name, input string, newline bool) error {
	resp, err := c.send(Request{
		Action:  "send",
		Name:    name,
		Input:   input,
		Newline: newline,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (c *Client) Stop(name string) error {
	resp, err := c.send(Request{
		Action: "stop",
		Name:   name,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (c *Client) Kill(name string) error {
	resp, err := c.send(Request{
		Action: "kill",
		Name:   name,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

type SearchRequest struct {
	Name       string
	Pattern    string
	Before     int
	After      int
	IgnoreCase bool
	StripANSI  bool
}

type SearchMatch struct {
	LineNumber int      `json:"line_number"`
	Line       string   `json:"line"`
	Before     []string `json:"before"`
	After      []string `json:"after"`
}

type SearchResponse struct {
	Matches      []SearchMatch `json:"matches"`
	TotalMatches int           `json:"total_matches"`
}

type InfoResponse struct {
	Name          string  `json:"name"`
	State         string  `json:"state"`
	PID           int     `json:"pid"`
	Command       string  `json:"command"`
	CreatedAt     string  `json:"created_at"`
	StoppedAt     string  `json:"stopped_at,omitempty"`
	BytesBuffered int64   `json:"bytes_buffered"`
	ReadPosition  int64   `json:"read_position"`
	Cols          int     `json:"cols"`
	Rows          int     `json:"rows"`
	Uptime        float64 `json:"uptime_seconds,omitempty"`
}

func (c *Client) Clear(name string) error {
	resp, err := c.send(Request{
		Action: "clear",
		Name:   name,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (c *Client) Resize(name string, cols, rows int) error {
	resp, err := c.send(Request{
		Action: "resize",
		Name:   name,
		Cols:   cols,
		Rows:   rows,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (c *Client) Info(name string) (*InfoResponse, error) {
	resp, err := c.send(Request{
		Action: "info",
		Name:   name,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, _ := json.Marshal(resp.Data)
	var result InfoResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) Search(req SearchRequest) (*SearchResponse, error) {
	resp, err := c.send(Request{
		Action:     "search",
		Name:       req.Name,
		Pattern:    req.Pattern,
		Before:     req.Before,
		After:      req.After,
		IgnoreCase: req.IgnoreCase,
		StripANSI:  req.StripANSI,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	data, _ := json.Marshal(resp.Data)
	var result SearchResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) send(req Request) (*Response, error) {
	conn, err := net.Dial("unix", SocketPath())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(ClientDeadline))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func extractMapData(resp *Response) (map[string]interface{}, error) {
	if resp.Data == nil {
		return nil, fmt.Errorf("response has no data")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format: %T", resp.Data)
	}
	return data, nil
}
