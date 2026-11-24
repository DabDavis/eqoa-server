package shardchat

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const deliveryTimeout = 2 * time.Second

var errUnavailable = errors.New("shard chat unavailable")

// One authenticated connection carries both directions. A tell's ID matches
// its receipt to the waiting sender. Neither messages nor receipts are replayed
// on reconnect: the old recipient may already have seen the message.
type frame struct {
	ID      uint64   `json:",omitempty"`
	Message *Message `json:",omitempty"`
	Receipt *Receipt `json:",omitempty"`
}

type Bridge struct {
	Home    Realm
	receive func(Message) Receipt

	mu      sync.Mutex
	conn    *websocket.Conn
	next    uint64
	pending map[uint64]chan Receipt
}

func New(home Realm, receive func(Message) Receipt) *Bridge {
	return &Bridge{Home: home, receive: receive, pending: make(map[uint64]chan Receipt)}
}

// Send returns immediately when disconnected. A shout completes on write, a
// tell when the recipient's server answers. No game/world lock is held here.
func (b *Bridge) Send(m Message) (Receipt, error) {
	b.mu.Lock()
	c := b.conn
	if c == nil || (m.Channel == "tell" && len(b.pending) >= 128) {
		b.mu.Unlock()
		return Receipt{}, errUnavailable
	}
	f := frame{Message: &m}
	var answer chan Receipt
	if m.Channel == "tell" {
		b.next++
		f.ID = b.next
		answer = make(chan Receipt, 1)
		b.pending[f.ID] = answer
	}
	b.mu.Unlock()
	if answer != nil {
		defer func() {
			b.mu.Lock()
			delete(b.pending, f.ID)
			b.mu.Unlock()
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()
	if err := wsjson.Write(ctx, c, f); err != nil {
		return Receipt{}, err
	}
	if answer == nil {
		return Receipt{}, nil
	}
	select {
	case r, ok := <-answer:
		if ok {
			return r, nil
		}
		return Receipt{}, errUnavailable
	case <-ctx.Done():
		return Receipt{}, ctx.Err()
	}
}

// Serve owns c until it closes or ctx ends. Incoming player text is passed
// straight to local delivery, never into a command parser or back to the peer.
func (b *Bridge) Serve(ctx context.Context, c *websocket.Conn) error {
	defer c.CloseNow()
	c.SetReadLimit(256 << 10) // one bounded chat line and up to four item snapshots
	b.mu.Lock()
	if b.conn != nil {
		b.mu.Unlock()
		return errors.New("shard chat already connected")
	}
	b.conn = c
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.conn = nil
		for id, ch := range b.pending {
			close(ch)
			delete(b.pending, id)
		}
		b.mu.Unlock()
	}()
	for {
		var f frame
		if err := wsjson.Read(ctx, c, &f); err != nil {
			return err
		}
		switch {
		case f.Message != nil && f.Receipt == nil:
			m := f.Message
			if !((m.Channel == "tell" && f.ID != 0 && m.To != "") ||
				(m.Channel == "shout" && f.ID == 0 && m.To == "")) {
				return errors.New("invalid shard chat destination")
			}
			r := b.receive(*m)
			if f.ID != 0 {
				writeCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
				err := wsjson.Write(writeCtx, c, frame{ID: f.ID, Receipt: &r})
				cancel()
				if err != nil {
					return err
				}
			}
		case f.Receipt != nil && f.Message == nil && f.ID != 0:
			b.mu.Lock()
			if ch := b.pending[f.ID]; ch != nil {
				ch <- *f.Receipt
				delete(b.pending, f.ID)
			}
			b.mu.Unlock()
		default:
			return errors.New("invalid shard chat frame")
		}
	}
}
