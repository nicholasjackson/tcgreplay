package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRelaySurvivesDaemonRestart keeps one client session open across a
// daemon restart: the relay reconnects, replays the handshake and tells
// the client the tools may have changed.
func TestRelaySurvivesDaemonRestart(t *testing.T) {
	dir := t.TempDir()
	c := config{dataDir: dir, cacheDir: t.TempDir(), seat: "p2", offline: true}

	startDaemon := func() (stop func()) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- runDaemon(ctx, c, false) }()
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(socketPath(c)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("daemon did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return func() {
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
	}

	stop := startDaemon()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newRelay(c, outW)
	r.spawn = func(config) error { return nil } // the test starts daemons itself
	go r.run(ctx, inR)

	lines := make(chan string, 100)
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	send := func(s string) {
		t.Helper()
		if _, err := inW.Write([]byte(s + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	// expect waits for a line containing want, skipping others.
	expect := func(want string) string {
		t.Helper()
		timeout := time.After(10 * time.Second)
		for {
			select {
			case l := <-lines:
				if strings.Contains(l, want) {
					return l
				}
			case <-timeout:
				t.Fatalf("no line containing %s", want)
			}
		}
	}
	sendPing := func(id int) {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": "ping", "arguments": map[string]any{}}})
		send(string(b))
	}
	expectPong := func(id int) {
		t.Helper()
		if l := expect(fmt.Sprintf(`"id":%d`, id)); !strings.Contains(l, "pong") {
			t.Fatalf("ping %d: %s", id, l)
		}
	}
	ping := func(id int) {
		t.Helper()
		sendPing(id)
		expectPong(id)
	}

	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	expect(`"id":1`)
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	ping(2)

	// Restart the daemon. A call made while it is down waits for the new one.
	stop()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		gone := r.conn == nil
		r.mu.Unlock()
		if gone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("relay did not notice the daemon stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		stop = startDaemon()
	}()
	sendPing(3)
	expect("notifications/tools/list_changed") // sent before the reply
	expectPong(3)
	ping(4)
	inW.Close()
	cancel()
	stop()
}
