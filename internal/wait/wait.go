package wait

import (
	"fmt"
	"regexp"
	"time"
)

const DefaultPollInterval = 50 * time.Millisecond

type ReadFunc func() (output string, position int, err error)

type Config struct {
	Pattern       string
	SettleMs      int
	TimeoutSec    int
	StartPosition int
	PollInterval  time.Duration
}

func ForOutput(readFn ReadFunc, cfg Config) (string, int, error) {
	var re *regexp.Regexp
	if cfg.Pattern != "" {
		var err error
		re, err = regexp.Compile(cfg.Pattern)
		if err != nil {
			return "", 0, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	deadline := time.Now().Add(timeout)
	settleDuration := time.Duration(cfg.SettleMs) * time.Millisecond

	lastPos := cfg.StartPosition
	lastChangeTime := time.Now()

	for time.Now().Before(deadline) {
		output, pos, err := readFn()
		if err != nil {
			return "", 0, err
		}

		if pos != lastPos {
			lastPos = pos
			lastChangeTime = time.Now()
		}

		newOutput := ""
		if pos > cfg.StartPosition {
			newOutput = output[cfg.StartPosition:]
		}

		if re != nil && re.MatchString(newOutput) {
			return newOutput, pos, nil
		}

		if cfg.SettleMs > 0 && pos > cfg.StartPosition && time.Since(lastChangeTime) >= settleDuration {
			return newOutput, pos, nil
		}

		time.Sleep(pollInterval)
	}

	output, pos, _ := readFn()
	newOutput := ""
	if pos > cfg.StartPosition {
		newOutput = output[cfg.StartPosition:]
	}

	if re != nil {
		return newOutput, pos, fmt.Errorf("timeout waiting for pattern %q", cfg.Pattern)
	}
	return newOutput, pos, fmt.Errorf("timeout waiting for output to settle")
}
