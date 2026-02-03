package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
)

func TestExecArgsValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      ExecArgs
		wantInput string
		wantErr   string
	}{
		{
			name:      "regular input",
			args:      ExecArgs{Name: "test", Input: "hello"},
			wantInput: "hello",
		},
		{
			name:      "input with special chars",
			args:      ExecArgs{Name: "test", Input: "print(\"hello\\nworld\")"},
			wantInput: "print(\"hello\\nworld\")",
		},
		{
			name:    "empty input",
			args:    ExecArgs{Name: "test"},
			wantErr: "input is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := validateExecInput(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input != tt.wantInput {
				t.Errorf("got input %q, want %q", input, tt.wantInput)
			}
		})
	}
}

func TestSendArgsBase64Decoding(t *testing.T) {
	tests := []struct {
		name        string
		args        SendArgs
		wantInput   string
		wantErr     string
	}{
		{
			name:      "regular input",
			args:      SendArgs{Name: "test", Input: "hello"},
			wantInput: "hello",
		},
		{
			name:      "base64 input",
			args:      SendArgs{Name: "test", InputBase64: base64.StdEncoding.EncodeToString([]byte("hello"))},
			wantInput: "hello",
		},
		{
			name:    "both input and input_base64",
			args:    SendArgs{Name: "test", Input: "hello", InputBase64: "aGVsbG8="},
			wantErr: "input and input_base64 are mutually exclusive",
		},
		{
			name:    "neither input nor input_base64",
			args:    SendArgs{Name: "test"},
			wantErr: "input or input_base64 is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := decodeSendInput(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input != tt.wantInput {
				t.Errorf("got input %q, want %q", input, tt.wantInput)
			}
		})
	}
}

func TestSendBase64RoundTrip(t *testing.T) {
	testCases := []string{
		"simple text",
		"print(\"hello\")",
		"SELECT * FROM users WHERE name = 'O\\'Brien'",
		"echo \"test\\n\\twith escapes\"",
		"\\x03", // ctrl+c as string
	}

	for _, input := range testCases {
		encoded := base64.StdEncoding.EncodeToString([]byte(input))
		args := SendArgs{Name: "test", InputBase64: encoded}
		decoded, err := decodeSendInput(args)
		if err != nil {
			t.Errorf("failed to decode %q: %v", input, err)
			continue
		}
		if decoded != input {
			t.Errorf("roundtrip failed: got %q, want %q", decoded, input)
		}
	}
}

// validateExecInput validates and returns input from ExecArgs
func validateExecInput(a ExecArgs) (string, error) {
	if a.Input == "" {
		return "", fmt.Errorf("input is required")
	}
	return a.Input, nil
}

// decodeSendInput extracts and decodes input from SendArgs
func decodeSendInput(a SendArgs) (string, error) {
	if a.Input == "" && a.InputBase64 == "" {
		return "", fmt.Errorf("input or input_base64 is required")
	}
	if a.InputBase64 != "" {
		if a.Input != "" {
			return "", fmt.Errorf("input and input_base64 are mutually exclusive")
		}
		decoded, err := base64.StdEncoding.DecodeString(a.InputBase64)
		if err != nil {
			return "", fmt.Errorf("decode input_base64: %w", err)
		}
		return string(decoded), nil
	}
	return a.Input, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Verify json unmarshaling works correctly
func TestExecArgsJSONUnmarshal(t *testing.T) {
	jsonStr := `{"name": "test", "input": "hello"}`
	var args ExecArgs
	if err := json.Unmarshal([]byte(jsonStr), &args); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if args.Input != "hello" {
		t.Errorf("got Input %q, want %q", args.Input, "hello")
	}
}

func TestSendArgsJSONUnmarshal(t *testing.T) {
	jsonStr := `{"name": "test", "input_base64": "aGVsbG8="}`
	var args SendArgs
	if err := json.Unmarshal([]byte(jsonStr), &args); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if args.InputBase64 != "aGVsbG8=" {
		t.Errorf("got InputBase64 %q, want %q", args.InputBase64, "aGVsbG8=")
	}
}
