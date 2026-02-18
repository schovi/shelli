package daemon

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) (*Client, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	storage := NewMemoryStorage(1024 * 1024)
	srv, err := NewServer(
		WithStorage(storage),
		WithSocketDir(tmpDir),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	sockPath := srv.socketPath()
	client := NewClientWithSocketPath(sockPath)

	deadline := time.Now().Add(2 * time.Second)
	for !client.Ping() {
		if time.Now().After(deadline) {
			t.Fatal("server did not start in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cleanup := func() {
		srv.Shutdown()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("server error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down in time")
		}
	}

	return client, cleanup
}

func waitForOutput(t *testing.T, client *Client, name string, contains string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output, _, err := client.Read(name, "all", 0, 0)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(output, contains) {
			return output
		}
		time.Sleep(50 * time.Millisecond)
	}
	output, _, _ := client.Read(name, "all", 0, 0)
	t.Fatalf("timed out waiting for %q in output, got: %q", contains, output)
	return ""
}

func TestLifecycle(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("create session", func(t *testing.T) {
		result, err := client.Create("test1", CreateOptions{Command: "sh"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if result["name"] != "test1" {
			t.Errorf("name = %v, want test1", result["name"])
		}
	})

	t.Run("send and read", func(t *testing.T) {
		if err := client.Send("test1", "echo hello-world", true); err != nil {
			t.Fatalf("send: %v", err)
		}
		waitForOutput(t, client, "test1", "hello-world")
	})

	t.Run("incremental read tracks position", func(t *testing.T) {
		output1, pos1, err := client.Read("test1", "new", 0, 0)
		if err != nil {
			t.Fatalf("read new: %v", err)
		}
		if pos1 == 0 {
			t.Error("position should be > 0 after reading")
		}

		output2, pos2, err := client.Read("test1", "new", 0, 0)
		if err != nil {
			t.Fatalf("second read: %v", err)
		}
		if pos2 < pos1 {
			t.Errorf("position should not decrease: %d < %d", pos2, pos1)
		}
		if len(output2) >= len(output1) && output1 != "" {
			t.Logf("second read returned more than expected: %q (first was %q)", output2, output1)
		}
	})

	t.Run("list shows session", func(t *testing.T) {
		sessions, err := client.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		found := false
		for _, s := range sessions {
			if s.Name == "test1" {
				found = true
				if s.State != "running" {
					t.Errorf("state = %s, want running", s.State)
				}
			}
		}
		if !found {
			t.Error("session test1 not found in list")
		}
	})

	t.Run("stop preserves output", func(t *testing.T) {
		if err := client.Stop("test1"); err != nil {
			t.Fatalf("stop: %v", err)
		}

		sessions, err := client.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, s := range sessions {
			if s.Name == "test1" && s.State != "stopped" {
				t.Errorf("state after stop = %s, want stopped", s.State)
			}
		}

		output, _, err := client.Read("test1", "all", 0, 0)
		if err != nil {
			t.Fatalf("read after stop: %v", err)
		}
		if !strings.Contains(output, "hello-world") {
			t.Errorf("output after stop should contain hello-world, got: %q", output)
		}
	})

	t.Run("send to stopped session fails", func(t *testing.T) {
		err := client.Send("test1", "should fail", true)
		if err == nil {
			t.Fatal("send to stopped session should fail")
		}
	})

	t.Run("kill removes session", func(t *testing.T) {
		if err := client.Kill("test1"); err != nil {
			t.Fatalf("kill: %v", err)
		}

		sessions, err := client.List()
		if err != nil {
			t.Fatalf("list after kill: %v", err)
		}
		for _, s := range sessions {
			if s.Name == "test1" {
				t.Error("session test1 should not exist after kill")
			}
		}

		_, _, err = client.Read("test1", "all", 0, 0)
		if err == nil {
			t.Error("read after kill should fail")
		}
	})
}

func TestSearchBoundsValidation(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.Create("search-test", CreateOptions{Command: "sh"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer client.Kill("search-test")

	if err := client.Send("search-test", "echo searchable-text", true); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForOutput(t, client, "search-test", "searchable-text")

	t.Run("negative before", func(t *testing.T) {
		_, err := client.Search(SearchRequest{
			Name:    "search-test",
			Pattern: "searchable",
			Before:  -1,
		})
		if err == nil {
			t.Fatal("negative before should produce error")
		}
		if !strings.Contains(err.Error(), "non-negative") {
			t.Errorf("error should mention non-negative, got: %v", err)
		}
	})

	t.Run("negative after", func(t *testing.T) {
		_, err := client.Search(SearchRequest{
			Name:    "search-test",
			Pattern: "searchable",
			After:   -5,
		})
		if err == nil {
			t.Fatal("negative after should produce error")
		}
	})

	t.Run("valid search works", func(t *testing.T) {
		result, err := client.Search(SearchRequest{
			Name:    "search-test",
			Pattern: "searchable",
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if result.TotalMatches == 0 {
			t.Error("expected at least one match")
		}
	})

	t.Run("search with context lines", func(t *testing.T) {
		result, err := client.Search(SearchRequest{
			Name:    "search-test",
			Pattern: "searchable",
			Before:  2,
			After:   2,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if result.TotalMatches == 0 {
			t.Error("expected at least one match")
		}
	})

	t.Run("nonexistent session", func(t *testing.T) {
		_, err := client.Search(SearchRequest{
			Name:    "nonexistent",
			Pattern: "test",
		})
		if err == nil {
			t.Fatal("search on nonexistent session should fail")
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	const numSessions = 5
	var wg sync.WaitGroup

	for i := range numSessions {
		name := "concurrent-" + string(rune('a'+i))
		_, err := client.Create(name, CreateOptions{Command: "sh"})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		defer client.Kill(name)
	}

	wg.Add(numSessions)
	errors := make(chan error, numSessions*10)

	for i := range numSessions {
		go func(idx int) {
			defer wg.Done()
			name := "concurrent-" + string(rune('a'+idx))

			if err := client.Send(name, "echo output-"+name, true); err != nil {
				errors <- err
				return
			}

			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				output, _, err := client.Read(name, "all", 0, 0)
				if err != nil {
					errors <- err
					return
				}
				if strings.Contains(output, "output-"+name) {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			errors <- nil
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("concurrent error: %v", err)
		}
	}

	sessions, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != numSessions {
		t.Errorf("expected %d sessions, got %d", numSessions, len(sessions))
	}
}

func TestPerCursorReads(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.Create("cursor-test", CreateOptions{Command: "sh"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer client.Kill("cursor-test")

	if err := client.Send("cursor-test", "echo first-line", true); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForOutput(t, client, "cursor-test", "first-line")

	out1, _, err := client.ReadWithCursor("cursor-test", "new", "consumer-a", 0, 0)
	if err != nil {
		t.Fatalf("read cursor-a: %v", err)
	}
	if !strings.Contains(out1, "first-line") {
		t.Errorf("cursor-a first read should contain first-line, got: %q", out1)
	}

	if err := client.Send("cursor-test", "echo second-line", true); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForOutput(t, client, "cursor-test", "second-line")

	outA, _, err := client.ReadWithCursor("cursor-test", "new", "consumer-a", 0, 0)
	if err != nil {
		t.Fatalf("read cursor-a second: %v", err)
	}
	if !strings.Contains(outA, "second-line") {
		t.Errorf("cursor-a should see second-line, got: %q", outA)
	}
	if strings.Contains(outA, "first-line") {
		t.Errorf("cursor-a should NOT see first-line again, got: %q", outA)
	}

	outB, _, err := client.ReadWithCursor("cursor-test", "new", "consumer-b", 0, 0)
	if err != nil {
		t.Fatalf("read cursor-b: %v", err)
	}
	if !strings.Contains(outB, "first-line") {
		t.Errorf("cursor-b should see first-line (fresh cursor), got: %q", outB)
	}
	if !strings.Contains(outB, "second-line") {
		t.Errorf("cursor-b should see second-line, got: %q", outB)
	}
}

func TestSessionErrorCases(t *testing.T) {
	client, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("create with invalid name", func(t *testing.T) {
		_, err := client.Create("invalid name!", CreateOptions{Command: "sh"})
		if err == nil {
			t.Fatal("create with invalid name should fail")
		}
	})

	t.Run("duplicate create", func(t *testing.T) {
		_, err := client.Create("dup-test", CreateOptions{Command: "sh"})
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		defer client.Kill("dup-test")

		_, err = client.Create("dup-test", CreateOptions{Command: "sh"})
		if err == nil {
			t.Fatal("duplicate create should fail")
		}
	})

	t.Run("operations on nonexistent session", func(t *testing.T) {
		if err := client.Send("nope", "test", true); err == nil {
			t.Error("send to nonexistent should fail")
		}
		if _, _, err := client.Read("nope", "all", 0, 0); err == nil {
			t.Error("read nonexistent should fail")
		}
		if err := client.Stop("nope"); err == nil {
			t.Error("stop nonexistent should fail")
		}
		if err := client.Kill("nope"); err == nil {
			t.Error("kill nonexistent should fail")
		}
	})

	t.Run("stop is idempotent", func(t *testing.T) {
		_, err := client.Create("stop-twice", CreateOptions{Command: "sh"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		defer client.Kill("stop-twice")

		if err := client.Stop("stop-twice"); err != nil {
			t.Fatalf("first stop: %v", err)
		}
		if err := client.Stop("stop-twice"); err != nil {
			t.Fatalf("second stop should succeed: %v", err)
		}
	})
}
