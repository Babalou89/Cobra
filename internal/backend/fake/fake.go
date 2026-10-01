// Package fake is a scripted backend.Backend for offline tests: it replays a
// list of replies (or a callback) so the real agent loop can run with zero
// tokens and no network.
package fake

import (
	"fmt"
	"sync"

	"cobra/internal/backend"
)

// Backend replays Replies in order. When the script runs out it returns an
// error unless Repeat is set, in which case the last reply repeats forever.
// Fn, when non-nil, takes precedence and is called with the 0-based call index.
type Backend struct {
	Replies []string
	Repeat  bool
	Fn      func(call int, msgs []backend.Msg) (string, error)
	Ctx     int // MaxContext; default 32768

	mu    sync.Mutex
	calls [][]backend.Msg
}

// New returns a Backend that replays the given replies once.
func New(replies ...string) *Backend { return &Backend{Replies: replies} }

// Chat records the request and returns the next scripted reply.
func (b *Backend) Chat(msgs []backend.Msg, _ float64, _ int) (string, error) {
	b.mu.Lock()
	i := len(b.calls)
	cp := make([]backend.Msg, len(msgs))
	copy(cp, msgs)
	b.calls = append(b.calls, cp)
	b.mu.Unlock()
	if b.Fn != nil {
		return b.Fn(i, msgs)
	}
	if i < len(b.Replies) {
		return b.Replies[i], nil
	}
	if b.Repeat && len(b.Replies) > 0 {
		return b.Replies[len(b.Replies)-1], nil
	}
	return "", fmt.Errorf("fake backend: script exhausted after %d replies", len(b.Replies))
}

// Calls returns how many times Chat was called.
func (b *Backend) Calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.calls)
}

// Request returns the messages of the i-th (0-based) Chat call.
func (b *Backend) Request(i int) []backend.Msg {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[i]
}

func (b *Backend) Health() bool { return true }
func (b *Backend) Name() string { return "fake" }
func (b *Backend) MaxContext() int {
	if b.Ctx > 0 {
		return b.Ctx
	}
	return 32768
}

var _ backend.Backend = (*Backend)(nil)
