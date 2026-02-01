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

func (c *Client) Create(name, command string) (map[string]interface{}, error) {
	resp, err := c.send(Request{
		Action:  "create",
		Name:    name,
		Command: command,
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

	data, _ := json.Marshal(resp.Data)
	var sessions []SessionInfo
	json.Unmarshal(data, &sessions)
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
