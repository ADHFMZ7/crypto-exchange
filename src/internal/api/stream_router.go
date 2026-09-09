package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
	"github.com/coder/websocket"
)

// StreamRouter serves the live market feed.
//
// Public, like the tape and the book it mirrors: a trade carries no order ids
// and no owners, so there is nothing here that belongs to an account. A private
// feed — your fills, your order status — would need authentication and is a
// different endpoint.
type StreamRouter struct {
	Services *services.Services
	Hub      *stream.Hub
}

func NewStreamRouter(service *services.Services, hub *stream.Hub) *StreamRouter {
	return &StreamRouter{Services: service, Hub: hub}
}

const (
	// How long a client may say nothing before it is assumed gone. A browser
	// tab that is closed without a close frame — a crash, a lost network — looks
	// identical to an idle one until a ping fails.
	pingInterval = 25 * time.Second

	// A write that cannot complete in this long means the connection is wedged,
	// not merely slow, and the hub has already stopped queueing for it.
	writeTimeout = 10 * time.Second
)

func (router *StreamRouter) Register(mux *http.ServeMux) {
	mux.Handle("GET /stream", http.HandlerFunc(router.Stream))
}

// clientMessage is everything a client may say. Today that is which markets it
// wants; the envelope exists so adding more does not become a second protocol.
type clientMessage struct {
	Type    string   `json:"type"`
	Markets []string `json:"markets"`
}

// Stream upgrades the connection and forwards market events until it closes.
//
// Two goroutines per connection, which is the shape the websocket library
// requires: one reader, because concurrent reads are not permitted, and the
// handler itself writing. The reader exists only to service subscriptions and
// to notice the connection dying.
func (router *StreamRouter) Stream(w http.ResponseWriter, r *http.Request) {

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The browser sends an Origin the library checks against the Host. The
		// dev server runs on a different port, so it has to be named — and
		// nothing here is authenticated, so a permissive origin grants access
		// to data that is already public.
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		// Accept has already written a response.
		return
	}
	defer conn.CloseNow()

	client := router.Hub.Subscribe()
	defer client.Close()

	// A client may name its markets up front, so the common case needs no
	// round trip before events start arriving.
	if markets := r.URL.Query().Get("markets"); markets != "" {
		client.Watch(splitMarkets(markets))
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go router.read(ctx, cancel, conn, client)

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-client.Done():
			// Dropped for falling behind. Say so rather than closing silently,
			// so the client knows its tape has a gap and can re-read it.
			conn.Close(websocket.StatusTryAgainLater, "fell behind the feed")
			return

		case event := <-client.Events():
			if err := writeWithin(ctx, conn, event); err != nil {
				return
			}

		case <-ticker.C:
			pingCtx, done := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			done()
			if err != nil {
				return
			}
		}
	}
}

// read services subscription messages and, more importantly, notices the
// connection ending — a websocket close is only observed by a reader.
func (router *StreamRouter) read(ctx context.Context, cancel context.CancelFunc,
	conn *websocket.Conn, client *stream.Client) {

	defer cancel()

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			var closeErr websocket.CloseError
			if !errors.As(err, &closeErr) && !errors.Is(err, context.Canceled) {
				log.Printf("stream: read: %v", err)
			}
			return
		}

		var message clientMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			continue // Not something we understand; ignore rather than disconnect.
		}
		if message.Type == "subscribe" {
			client.Watch(message.Markets)
		}
	}
}

func writeWithin(ctx context.Context, conn *websocket.Conn, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	return conn.Write(ctx, websocket.MessageText, payload)
}

func splitMarkets(raw string) []string {
	parts := strings.Split(raw, ",")
	markets := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			markets = append(markets, trimmed)
		}
	}
	return markets
}
