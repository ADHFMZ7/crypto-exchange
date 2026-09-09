// Package stream fans market events out to connected clients.
//
// The exchange's own state lives in Postgres and in the matching engine; this
// is only a delivery mechanism on top of it. Nothing here is authoritative, and
// nothing here may slow down the two things that are.
package stream

import (
	"encoding/json"
	"sync"
)

// Event is one thing that happened on a market, as a client receives it.
//
// The envelope carries a type so the same connection can grow more kinds of
// event without the client guessing from shape.
type Event struct {
	Type    string `json:"type"`
	Market  string `json:"market"`
	Payload any    `json:"payload,omitempty"`
}

// Event types. Trades are the only one today; the envelope exists so adding
// depth or ticker updates does not become a second protocol.
const (
	EventTrade = "trade"
)

// backlog is how many events a client may fall behind by before it is dropped.
//
// Generous enough that a garbage collection pause or a slow paint does not cost
// a connection, small enough that a client which has genuinely stopped reading
// cannot make the server hold megabytes on its behalf.
const backlog = 64

// Hub is the set of currently connected clients.
//
// It is deliberately not durable and not ordered across markets: a client that
// misses an event reconnects and re-reads the REST endpoint, which is the
// authority. Treating this as a source of truth is the mistake it is shaped to
// prevent.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

// Client is one subscriber's view of the hub.
type Client struct {
	hub *Hub

	// Buffered, so Publish never waits on a reader.
	//
	// Deliberately never closed. Publish sends to it after releasing the hub
	// lock, so closing it here would race a send already in flight, and a send
	// on a closed channel is a panic rather than an error. `done` carries the
	// end-of-stream signal instead, and the channel is collected with the
	// client.
	events chan []byte

	// Closed exactly once, by drop, which also removes the client from the hub.
	done chan struct{}

	mu      sync.RWMutex
	markets map[string]bool // empty means every market

	once sync.Once
}

// Subscribe registers a client. Cancel it with Close when the connection ends.
func (h *Hub) Subscribe() *Client {
	client := &Client{
		hub:     h,
		events:  make(chan []byte, backlog),
		done:    make(chan struct{}),
		markets: map[string]bool{},
	}

	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	return client
}

// Publish delivers an event to every client watching that market.
//
// It never blocks. A client that has not kept up is dropped rather than waited
// for, because the alternative is letting one stalled browser tab apply
// backpressure all the way into settlement. The dropped client's connection
// closes, it reconnects, and it re-reads the tape over REST — a gap it can
// recover from, unlike a ledger that stopped recording.
func (h *Hub) Publish(event Event) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		if client.watching(event.Market) {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range clients {
		select {
		case client.events <- encoded:
		case <-client.done:
			// Gone between the snapshot above and now. Nothing to deliver to.
		default:
			client.drop()
		}
	}
}

// Clients reports how many connections are attached, for tests and diagnostics.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Events is the stream of encoded events for this client.
//
// It is never closed, so a reader must select on Done as well — otherwise it
// blocks forever once the client is dropped.
func (c *Client) Events() <-chan []byte { return c.events }

// Done closes when this client is detached, whether by its own Close or by
// falling too far behind.
func (c *Client) Done() <-chan struct{} { return c.done }

// Watch replaces the set of markets this client receives.
//
// An empty list means every market, which is what a client that has connected
// but not yet said what it wants gets — a tape with everything on it is a more
// useful default than silence.
func (c *Client) Watch(markets []string) {
	next := make(map[string]bool, len(markets))
	for _, market := range markets {
		if market != "" {
			next[market] = true
		}
	}

	c.mu.Lock()
	c.markets = next
	c.mu.Unlock()
}

func (c *Client) watching(market string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.markets) == 0 || c.markets[market]
}

// Close detaches the client. Safe to call more than once, and safe to call
// while Publish is running.
func (c *Client) Close() { c.drop() }

func (c *Client) drop() {
	c.once.Do(func() {
		c.hub.mu.Lock()
		delete(c.hub.clients, c)
		c.hub.mu.Unlock()

		close(c.done)
	})
}
