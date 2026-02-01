package daemon

import (
	"strings"
	"testing"
)

func TestValidateSessionName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
		errMsg  string
	}{
		{"foo", false, ""},
		{"my-session", false, ""},
		{"test_123", false, ""},
		{"a.b.c", false, ""},
		{"A1", false, ""},
		{"session-with-dots.and_underscores", false, ""},

		{"", true, "cannot be empty"},
		{"../etc", true, "must start with alphanumeric"},
		{"-start", true, "must start with alphanumeric"},
		{"a/b", true, "must start with alphanumeric"},
		{"a b", true, "must start with alphanumeric"},
		{".hidden", true, "must start with alphanumeric"},
		{"_underscore", true, "must start with alphanumeric"},
		{strings.Repeat("a", 65), true, "too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionName(tt.name)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSessionName(%q) = nil, want error containing %q", tt.name, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateSessionName(%q) = %v, want error containing %q", tt.name, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSessionName(%q) = %v, want nil", tt.name, err)
				}
			}
		})
	}
}

func TestValidateSessionName_MaxLength(t *testing.T) {
	validName := strings.Repeat("a", 64)
	if err := ValidateSessionName(validName); err != nil {
		t.Errorf("64-char name should be valid, got %v", err)
	}

	invalidName := strings.Repeat("a", 65)
	if err := ValidateSessionName(invalidName); err == nil {
		t.Error("65-char name should be invalid")
	}
}
