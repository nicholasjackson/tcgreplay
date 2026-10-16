package webui

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// maxFrames caps how many per-event frames one push carries; larger
// updates (e.g. setup) jump straight to the final board.
const maxFrames = 40

// hub tracks connected clients and pushes snapshots after every change.
type hub struct {
	s       *Server
	mu      sync.Mutex
	clients map[*client]struct{}
	watched map[*game.Game]bool
	pending []int // events since the last push, for frames
	signal  chan struct{}
}

type client struct {
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.Mutex
	reveal bool
}

func newHub(s *Server) *hub {
	return &hub{
		s:       s,
		clients: map[*client]struct{}{},
		watched: map[*game.Game]bool{},
		signal:  make(chan struct{}, 1),
	}
}

// watch subscribes to a game's updates. Game listeners run while the game
// is locked, so this only records the events and wakes the push loop.
func (h *hub) watch(g *game.Game) {
	h.mu.Lock()
	first := !h.watched[g]
	h.watched[g] = true
	h.pending = nil
	h.mu.Unlock()
	if first {
		g.Subscribe(func(u game.Update) {
			h.mu.Lock()
			h.pending = append(h.pending, u.Events...)
			h.mu.Unlock()
			h.wake()
		})
	}
	h.wake()
}

func (h *hub) wake() {
	select {
	case h.signal <- struct{}{}:
	default:
	}
}

func (h *hub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.signal:
			h.mu.Lock()
			events := h.pending
			h.pending = nil
			clients := make([]*client, 0, len(h.clients))
			for c := range h.clients {
				clients = append(clients, c)
			}
			h.mu.Unlock()
			if len(events) > maxFrames {
				events = nil
			}
			cache := map[bool][]byte{}
			for _, c := range clients {
				c.mu.Lock()
				reveal := c.reveal
				c.mu.Unlock()
				msg, ok := cache[reveal]
				if !ok {
					var fellBack bool
					msg, fellBack = h.stateMessage(reveal, events)
					if fellBack {
						c.mu.Lock()
						c.reveal = false
						c.mu.Unlock()
					} else {
						cache[reveal] = msg
					}
				}
				c.push(msg)
			}
		}
	}
}

// stateMessage builds a "state" message. If the reveal view is no longer
// allowed it falls back to the seat view and reports that.
func (h *hub) stateMessage(reveal bool, frames []int) ([]byte, bool) {
	msg := map[string]any{
		"type":   "state",
		"human":  h.s.Human,
		"claude": h.s.Human.Other(),
		"reveal": reveal,
	}
	g, err := h.s.M.Current()
	if err != nil {
		games, _ := h.s.M.List()
		msg["snapshot"] = nil
		msg["games"] = games
		b, _ := json.Marshal(msg)
		return b, false
	}
	viewer := h.s.Human
	if reveal {
		viewer = ""
	}
	snap, err := g.UISnapshot(viewer, frames)
	fellBack := false
	if err != nil && reveal {
		fellBack = true
		msg["reveal"] = false
		snap, err = g.UISnapshot(h.s.Human, frames)
	}
	if err != nil {
		msg["error"] = err.Error()
	}
	msg["snapshot"] = snap
	b, _ := json.Marshal(msg)
	return b, fellBack
}

func (c *client) push(b []byte) {
	select {
	case c.send <- b:
	default:
		// A client this far behind is gone; it reconnects and reloads.
		c.conn.CloseNow()
	}
}

func (h *hub) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*", "[::1]:*"},
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	c := &client{conn: conn, send: make(chan []byte, 32)}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		conn.CloseNow()
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-c.send:
				wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, b)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Load state on connect.
	msg, _ := h.stateMessage(false, nil)
	c.push(msg)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var req request
		if err := json.Unmarshal(data, &req); err != nil {
			c.push(mustJSON(response{Type: "result", Error: "bad request: " + err.Error()}))
			continue
		}
		res := h.s.dispatch(r.Context(), c, req)
		c.push(mustJSON(res))
		if res.refresh {
			h.wake()
		}
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("webui: marshal: %v", err)
		return []byte(`{"type":"result","error":"internal error"}`)
	}
	return b
}

// seatOf parses a seat argument, defaulting to def.
func seatOf(s string, def state.Seat) state.Seat {
	if s == "" {
		return def
	}
	return state.Seat(s)
}
