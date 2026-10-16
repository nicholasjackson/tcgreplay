package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// runShim relays stdio to the daemon, starting or replacing it first.
func runShim(ctx context.Context, c config) error {
	conn, err := ensureDaemon(ctx, c)
	if err != nil {
		return err
	}
	defer conn.Close()
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, os.Stdin)
		// The client closed stdin: let the daemon end the session.
		if uc, ok := conn.(*net.UnixConn); ok {
			uc.CloseWrite()
		}
		done <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return nil
	}
}

// ensureDaemon connects to a daemon built from this binary, starting one
// when none runs and replacing one left over from an older build.
func ensureDaemon(ctx context.Context, c config) (net.Conn, error) {
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
	if err := spawnDaemon(c); err != nil {
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
