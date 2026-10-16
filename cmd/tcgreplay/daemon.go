package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// daemonInfo is written next to the socket so shims can tell whether the
// running daemon comes from the same binary as themselves.
type daemonInfo struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
	Binary  string `json:"binary"`
}

// socketPath lives in the data dir, unless that path would exceed the
// Unix socket limit (~108 bytes); then a short name in the runtime dir.
func socketPath(c config) string {
	p := filepath.Join(c.dataDir, "mcp.sock")
	if len(p) < 100 {
		return p
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	abs, _ := filepath.Abs(c.dataDir)
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(dir, fmt.Sprintf("tcgreplay-%x.sock", sum[:8]))
}

func infoPath(c config) string { return filepath.Join(c.dataDir, "daemon.json") }
func lockPath(c config) string { return filepath.Join(c.dataDir, "daemon.lock") }

// binaryID identifies the executable on disk, so a reinstall (go install)
// is noticed even when the version string is unchanged.
func binaryID() string {
	exe, err := os.Executable()
	if err != nil {
		return version
	}
	fi, err := os.Stat(exe)
	if err != nil {
		return version
	}
	return fmt.Sprintf("%s|%d|%d", exe, fi.Size(), fi.ModTime().UnixNano())
}

func readInfo(c config) (daemonInfo, error) {
	var d daemonInfo
	b, err := os.ReadFile(infoPath(c))
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(b, &d)
}

// runDaemon serves MCP sessions on a Unix socket, one per shim connection,
// all sharing one game manager and the playmat UI.
func runDaemon(ctx context.Context, c config) error {
	if err := os.MkdirAll(c.dataDir, 0o755); err != nil {
		return err
	}
	// The lock makes daemons mutually exclusive; it is released when the
	// process exits, however it exits.
	lock, err := os.OpenFile(lockPath(c), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Printf("daemon already running (%v); exiting", err)
		return nil
	}

	sock := socketPath(c)
	os.Remove(sock) // stale from a daemon that died; we hold the lock
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	defer os.Remove(sock)
	b, _ := json.Marshal(daemonInfo{PID: os.Getpid(), Version: version, Binary: binaryID()})
	if err := os.WriteFile(infoPath(c), b, 0o644); err != nil {
		return err
	}
	defer os.Remove(infoPath(c))

	m, srv := newServer(c)
	log.Printf("tcgreplay %s daemon (pid %d): socket %s, data %s, claude plays %s", version, os.Getpid(), sock, c.dataDir, c.seat)
	if c.uiAddr != "" {
		go keepUI(ctx, m, state.Seat(c.seat).Other(), c.uiAddr)
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for n := 1; ; n++ {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				log.Printf("daemon stopping")
				return nil
			}
			return err
		}
		go func() {
			ss, err := srv.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
			if err != nil {
				log.Printf("mcp session: %v", err)
				conn.Close()
				return
			}
			log.Printf("mcp session %d connected", n)
			ss.Wait()
			log.Printf("mcp session %d closed", n)
		}()
	}
}

// keepUI serves the playmat, retrying while the port is taken (e.g. by an
// older in-process server that has not exited yet), since the daemon is
// long-lived.
func keepUI(ctx context.Context, m *game.Manager, human state.Seat, addr string) {
	for warned := false; ; warned = true {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			log.Printf("playmat UI at http://%s/ (human plays %s)", ln.Addr(), human)
			if err := webui.New(m, human).Run(ctx, ln); err != nil && ctx.Err() == nil {
				log.Printf("playmat UI stopped: %v", err)
			}
			return
		}
		if !warned {
			log.Printf("playmat UI waiting for %s: %v", addr, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
