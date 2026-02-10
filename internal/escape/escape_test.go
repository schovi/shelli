package escape

import (
	"testing"
)

func TestInterpret_KnownSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"newline", `\n`, "\n"},
		{"carriage return", `\r`, "\r"},
		{"tab", `\t`, "\t"},
		{"escape", `\e`, "\x1b"},
		{"backslash", `\\`, "\\"},
		{"null", `\0`, "\x00"},
		{"hex ctrl-c", `\x03`, "\x03"},
		{"hex ctrl-d", `\x04`, "\x04"},
		{"hex uppercase", `\xFF`, "\xff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpret(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInterpret_UnknownSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"exclamation", `\!`, "!"},
		{"question mark", `\?`, "?"},
		{"slash", `\/`, "/"},
		{"at sign", `\@`, "@"},
		{"hash", `\#`, "#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpret(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInterpret_MixedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"text with known", `hello\nworld`, "hello\nworld"},
		{"text with unknown", `hello\!world`, "hello!world"},
		{"multiple escapes", `\t\n\r`, "\t\n\r"},
		{"known and unknown mixed", `\n\!\t\?`, "\n!\t?"},
		{"plain text only", "no escapes here", "no escapes here"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpret(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Interpret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInterpret_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"trailing backslash", `hello\`},
		{"bad hex digits", `\xZZ`},
		{"incomplete hex", `\x0`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Interpret(tt.input)
			if err == nil {
				t.Fatalf("expected error for input %q, got none", tt.input)
			}
		})
	}
}
