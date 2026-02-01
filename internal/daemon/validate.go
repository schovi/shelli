package daemon

import (
	"fmt"
	"regexp"
)

var validSessionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

const maxSessionNameLen = 64

func ValidateSessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if len(name) > maxSessionNameLen {
		return fmt.Errorf("session name too long (max %d chars)", maxSessionNameLen)
	}
	if !validSessionName.MatchString(name) {
		return fmt.Errorf("session name must start with alphanumeric and contain only letters, numbers, dots, dashes, or underscores")
	}
	return nil
}
