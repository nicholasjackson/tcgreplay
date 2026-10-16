package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"
)

// runShim relays stdio to the daemon, starting or replacing it first.
func runShim(ctx context.Context, c config) error {
	return newRelay(c, os.Stdout).run(ctx, os.Stdin)
}

// relay carries newline-delimited JSON-RPC between the MCP client and the
// daemon. It outlives any one daemon: when the daemon goes away (a rebuild,
// make dev), it connects to its successor and replays the client's MCP
// handshake there, so the client never sees the server disconnect.
type relay struct {
	c   config
	out io.Writer
	// spawn starts a daemon when none answers; tests replace it.
	spawn func(config) error

	outMu  sync.Mutex // serialises writes to out
	dialMu sync.Mutex // one reconnect at a time

	mu          sync.Mutex
	conn        net.Conn
	connected   int    // daemons connected so far
	init        []byte // the client's initialize request
	initID      string
	initialized []byte                     // the client's initialized notification
	pending     map[string]json.RawMessage // requests sent, not yet answered
}

func newRelay(c config, out io.Writer) *relay {
	return &relay{c: c, out: out, spawn: spawnDaemon, pending: map[string]json.RawMessage{}}
}

// rpcHead is the part of a JSON-RPC message the relay looks at.
type rpcHead struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func (r *relay) run(ctx context.Context, in io.Reader) error {
	if _, err := r.current(ctx); err != nil {
		return err
	}
	br := bufio.NewReader(in)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if serr := r.send(ctx, line); serr != nil {
				return serr
			}
		}
		if err != nil {
			// The client closed stdin: let the daemon end the session.
			r.mu.Lock()
			if uc, ok := r.conn.(*net.UnixConn); ok {
				uc.CloseWrite()
			}
			r.mu.Unlock()
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// send forwards one client message, reconnecting as often as needed.
func (r *relay) send(ctx context.Context, line []byte) error {
	var h rpcHead
	json.Unmarshal(line, &h)
	isRequest := h.Method != "" && len(h.ID) > 0
	r.mu.Lock()
	switch h.Method {
	case "initialize":
		r.init, r.initID = slices.Clone(line), string(h.ID)
	case "notifications/initialized":
		r.initialized = slices.Clone(line)
	}
	r.mu.Unlock()
	for {
		conn, err := r.current(ctx)
		if err != nil {
			return err
		}
		if isRequest {
			r.mu.Lock()
			r.pending[string(h.ID)] = h.ID
			r.mu.Unlock()
		}
		if _, err := conn.Write(line); err == nil {
			return nil
		}
		if isRequest {
			r.mu.Lock()
			delete(r.pending, string(h.ID)) // never delivered: resend it
			r.mu.Unlock()
		}
		r.broken(ctx, conn)
	}
}

// current returns the daemon connection, connecting (and replaying the
// handshake) if there is none. It waits until a daemon answers or ctx ends.
func (r *relay) current(ctx context.Context) (net.Conn, error) {
	r.dialMu.Lock()
	defer r.dialMu.Unlock()
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn != nil {
		return conn, nil
	}
	for {
		conn, br, err := r.connect(ctx)
		if err == nil {
			r.mu.Lock()
			r.conn = conn
			r.connected++
			again := r.connected > 1
			r.mu.Unlock()
			go r.read(ctx, conn, br)
			if again {
				log.Printf("reconnected to the daemon")
				// A new build may serve different tools.
				r.write([]byte(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}` + "\n"))
			}
			return conn, nil
		}
		log.Printf("daemon unavailable, retrying: %v", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// connect reaches a daemon and, if the client has already initialised,
// replays its handshake, swallowing the daemon's reply.
func (r *relay) connect(ctx context.Context) (net.Conn, *bufio.Reader, error) {
	r.mu.Lock()
	again := r.connected > 0
	r.mu.Unlock()
	if again {
		// A daemon that went away is usually being replaced (make dev):
		// give the successor a moment rather than racing it with our own.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if c, err := net.Dial("unix", socketPath(r.c)); err == nil {
				c.Close()
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	conn, err := ensureDaemon(ctx, r.c, r.spawn)
	if err != nil {
		return nil, nil, err
	}
	br := bufio.NewReader(conn)
	r.mu.Lock()
	init, initID, initialized := r.init, r.initID, r.initialized
	r.mu.Unlock()
	if init == nil {
		return conn, br, nil
	}
	fail := func(err error) (net.Conn, *bufio.Reader, error) {
		conn.Close()
		return nil, nil, fmt.Errorf("replaying the handshake: %w", err)
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(init); err != nil {
		return fail(err)
	}
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return fail(err)
		}
		var h rpcHead
		if json.Unmarshal(line, &h) == nil && h.Method == "" && string(h.ID) == initID {
			break
		}
	}
	if initialized != nil {
		if _, err := conn.Write(initialized); err != nil {
			return fail(err)
		}
	}
	conn.SetDeadline(time.Time{})
	return conn, br, nil
}

// read copies daemon messages to the client until the connection drops.
func (r *relay) read(ctx context.Context, conn net.Conn, br *bufio.Reader) {
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var h rpcHead
			if json.Unmarshal(line, &h) == nil && h.Method == "" && len(h.ID) > 0 {
				r.mu.Lock()
				delete(r.pending, string(h.ID))
				r.mu.Unlock()
			}
			r.write(line)
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("daemon connection lost: %v", err)
				r.broken(ctx, conn)
			}
			return
		}
	}
}

// broken drops a dead connection, fails the requests it was serving and
// reconnects in the background.
func (r *relay) broken(ctx context.Context, conn net.Conn) {
	r.mu.Lock()
	if r.conn != conn {
		r.mu.Unlock()
		return // already handled
	}
	r.conn = nil
	conn.Close()
	ids := slices.Collect(maps.Values(r.pending))
	clear(r.pending)
	r.mu.Unlock()
	for _, id := range ids {
		// The daemon may or may not have applied it, so it is not resent.
		msg, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32000, "message": "the tcgreplay daemon restarted during this call; check the state and retry"},
		})
		r.write(append(msg, '\n'))
	}
	go r.current(ctx)
}

func (r *relay) write(line []byte) {
	r.outMu.Lock()
	defer r.outMu.Unlock()
	r.out.Write(line)
}

// ensureDaemon connects to a daemon built from this binary, starting one
// when none runs and replacing one left over from an older build.
func ensureDaemon(ctx context.Context, c config, spawn func(config) error) (net.Conn, error) {
	if conn, err := net.Dial("unix", socketPath(c)); err == nil {
		info, err := readInfo(c)
		if err == nil && info.Binary == binaryID() {
			return conn, nil
		}
		conn.Close()
		if err == nil {
			log.Printf("replacing daemon pid %d from another build", info.PID)
			stopDaemon(ctx, c, info.PID)
		}
	}
	if err := spawn(c); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.Dial("unix", socketPath(c))
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("daemon did not start: %w (see %s)", err, c.dataDir+"/tcgreplay.log")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// stopDaemon asks the old daemon to exit and waits for its socket to go.
func stopDaemon(ctx context.Context, c config, pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Signal(syscall.SIGTERM)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if _, err := os.Stat(socketPath(c)); errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// spawnDaemon starts the daemon detached from this process, so it outlives
// the shim and the app that launched it.
func spawnDaemon(c config) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append([]string{"-daemon"}, c.args()...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}
	log.Printf("started daemon pid %d", cmd.Process.Pid)
	return cmd.Process.Release()
}
