package wait

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestForOutput_PatternMatch(t *testing.T) {
	output := "loading...\nready\n"
	readFn := func() (string, int, error) {
		return output, len(output), nil
	}

	cfg := Config{
		Pattern:       "ready",
		TimeoutSec:    1,
		StartPosition: 0,
	}

	got, pos, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "ready") {
		t.Errorf("expected output to contain 'ready', got %q", got)
	}
	if pos != len(output) {
		t.Errorf("expected position %d, got %d", len(output), pos)
	}
}

func TestForOutput_PatternTimeout(t *testing.T) {
	readFn := func() (string, int, error) {
		return "waiting...", 10, nil
	}

	cfg := Config{
		Pattern:       "never-match",
		TimeoutSec:    1,
		StartPosition: 0,
		PollInterval:  10 * time.Millisecond,
	}

	_, _, err := ForOutput(readFn, cfg)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func TestForOutput_SettleMode(t *testing.T) {
	callCount := 0
	readFn := func() (string, int, error) {
		callCount++
		return "output", 6, nil
	}

	cfg := Config{
		SettleMs:      50,
		TimeoutSec:    2,
		StartPosition: 0,
		PollInterval:  10 * time.Millisecond,
	}

	got, pos, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "output" {
		t.Errorf("expected 'output', got %q", got)
	}
	if pos != 6 {
		t.Errorf("expected position 6, got %d", pos)
	}
}

func TestForOutput_InvalidPattern(t *testing.T) {
	readFn := func() (string, int, error) {
		return "test", 4, nil
	}

	cfg := Config{
		Pattern:    "[invalid",
		TimeoutSec: 1,
	}

	_, _, err := ForOutput(readFn, cfg)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("expected 'invalid pattern' error, got %v", err)
	}
}

func TestForOutput_ReadError(t *testing.T) {
	readErr := errors.New("read failed")
	readFn := func() (string, int, error) {
		return "", 0, readErr
	}

	cfg := Config{
		Pattern:    "test",
		TimeoutSec: 1,
	}

	_, _, err := ForOutput(readFn, cfg)
	if !errors.Is(err, readErr) {
		t.Errorf("expected read error, got %v", err)
	}
}

func TestForOutput_StartPositionOffset(t *testing.T) {
	output := "old outputnew output"
	readFn := func() (string, int, error) {
		return output, len(output), nil
	}

	cfg := Config{
		Pattern:       "new",
		TimeoutSec:    1,
		StartPosition: 10,
	}

	got, _, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "new output" {
		t.Errorf("expected 'new output', got %q", got)
	}
}

func TestForOutput_DefaultPollInterval(t *testing.T) {
	if DefaultPollInterval != 50*time.Millisecond {
		t.Errorf("expected default poll interval 50ms, got %v", DefaultPollInterval)
	}
}

func TestForOutput_StartPositionExceedsOutput_NoPanic(t *testing.T) {
	readFn := func() (string, int, error) {
		return "hello", 100, nil
	}

	cfg := Config{
		SettleMs:      50,
		TimeoutSec:    1,
		StartPosition: 50,
		PollInterval:  10 * time.Millisecond,
	}

	got, pos, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error (should not panic): %v", err)
	}
	if got != "" {
		t.Errorf("expected empty output when StartPosition exceeds length, got %q", got)
	}
	if pos != 100 {
		t.Errorf("expected position 100, got %d", pos)
	}
}

func TestForOutput_PositionRegression_Settle(t *testing.T) {
	callCount := 0
	readFn := func() (string, int, error) {
		callCount++
		if callCount <= 2 {
			return strings.Repeat("x", 1000), 1000, nil
		}
		return "new content after truncation", 20, nil
	}

	cfg := Config{
		SettleMs:      50,
		TimeoutSec:    2,
		StartPosition: 1000,
		PollInterval:  10 * time.Millisecond,
	}

	got, pos, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("expected success after position regression, got error: %v", err)
	}
	if pos != 20 {
		t.Errorf("expected position 20, got %d", pos)
	}
	if got != "new content after truncation" {
		t.Errorf("expected truncated output, got %q", got)
	}
}

func TestForOutput_PositionRegression_Pattern(t *testing.T) {
	callCount := 0
	readFn := func() (string, int, error) {
		callCount++
		if callCount <= 2 {
			return strings.Repeat("x", 1000), 1000, nil
		}
		return "prompt> ready", 13, nil
	}

	cfg := Config{
		Pattern:       "ready",
		TimeoutSec:    2,
		StartPosition: 1000,
		PollInterval:  10 * time.Millisecond,
	}

	got, pos, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("expected pattern match after position regression, got error: %v", err)
	}
	if pos != 13 {
		t.Errorf("expected position 13, got %d", pos)
	}
	if !strings.Contains(got, "ready") {
		t.Errorf("expected output containing 'ready', got %q", got)
	}
}

func TestForOutput_PatternMatchWithLargeStartPosition(t *testing.T) {
	callCount := 0
	readFn := func() (string, int, error) {
		callCount++
		return "line1\nline2\nready\n", 18, nil
	}
	cfg := Config{
		Pattern:       "ready",
		TimeoutSec:    1,
		StartPosition: 0,
	}
	got, _, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "ready") {
		t.Errorf("expected output containing 'ready', got %q", got)
	}
}

func TestForOutput_SizeFunc_SkipsReadWhenUnchanged(t *testing.T) {
	readCount := 0
	readFn := func() (string, int, error) {
		readCount++
		return "output ready", 12, nil
	}
	sizeFunc := func() (int, error) {
		return 12, nil
	}
	cfg := Config{
		Pattern:       "ready",
		TimeoutSec:    1,
		StartPosition: 0,
		PollInterval:  10 * time.Millisecond,
		SizeFunc:      sizeFunc,
	}
	_, _, err := ForOutput(readFn, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readCount > 2 {
		t.Errorf("expected readFn called <=2 times, got %d", readCount)
	}
}
