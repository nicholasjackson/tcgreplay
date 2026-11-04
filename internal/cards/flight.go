package cards

import (
	"context"
	"sync"
	"time"
)

// flightGroup runs one piece of work per key at a time: callers asking
// for a key whose work is already running wait for that work instead of
// starting their own.
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	val  any
	err  error
}

// do runs fn for key, or joins the run already in progress, and returns
// its result. fn runs on a context detached from every caller and bounded
// by timeout (unbounded when zero), so a caller giving up on its own ctx
// returns early without cancelling the work for the others.
func (g *flightGroup) do(ctx context.Context, key string, timeout time.Duration, fn func(context.Context) (any, error)) (any, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]*flightCall{}
	}
	call, ok := g.calls[key]
	if !ok {
		call = &flightCall{done: make(chan struct{})}
		g.calls[key] = call
		go func() {
			fctx, cancel := context.Background(), context.CancelFunc(func() {})
			if timeout > 0 {
				fctx, cancel = context.WithTimeout(fctx, timeout)
			}
			defer cancel()
			call.val, call.err = fn(fctx)
			g.mu.Lock()
			delete(g.calls, key)
			g.mu.Unlock()
			close(call.done)
		}()
	}
	g.mu.Unlock()
	select {
	case <-call.done:
		return call.val, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
