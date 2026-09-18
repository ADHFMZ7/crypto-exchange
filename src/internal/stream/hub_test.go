package stream

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

/*
The hub's job is to deliver events without ever becoming something the exchange
waits on. So the tests are mostly about what it does when a client misbehaves:
one browser tab that has stopped reading must not be able to apply backpressure
through settlement and into the matching engine.
*/

func receive(t *testing.T, client *Client) Event {
	t.Helper()

	select {
	case raw := <-client.Events():
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("event is not JSON: %v", err)
		}
		return event
	case <-client.Done():
		t.Fatal("client was dropped while waiting for an event")
	case <-time.After(2 * time.Second):
		t.Fatal("no event arrived")
	}
	return Event{}
}

func expectNothing(t *testing.T, client *Client) {
	t.Helper()

	select {
	case raw := <-client.Events():
		t.Fatalf("unexpected event: %s", raw)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPublishReachesEverySubscriber(t *testing.T) {
	hub := NewHub()
	a, b := hub.Subscribe(), hub.Subscribe()
	defer a.Close()
	defer b.Close()

	if hub.Clients() != 2 {
		t.Fatalf("Clients() = %d, want 2", hub.Clients())
	}

	hub.Publish(Event{Type: EventTrade, Market: "BTC-USD", Payload: map[string]any{"price": 1}})

	for name, client := range map[string]*Client{"a": a, "b": b} {
		event := receive(t, client)
		if event.Type != EventTrade || event.Market != "BTC-USD" {
			t.Errorf("%s got %+v", name, event)
		}
	}
}

// A client that has said nothing yet gets everything: a tape with all markets
// on it is more useful than silence while the client decides.
func TestANewClientWatchesEveryMarket(t *testing.T) {
	hub := NewHub()
	client := hub.Subscribe()
	defer client.Close()

	hub.Publish(Event{Type: EventTrade, Market: "ETH-USD"})
	if event := receive(t, client); event.Market != "ETH-USD" {
		t.Fatalf("market = %q", event.Market)
	}
}

func TestWatchFiltersToTheChosenMarkets(t *testing.T) {
	hub := NewHub()
	client := hub.Subscribe()
	defer client.Close()

	client.Watch([]string{"BTC-USD"})

	hub.Publish(Event{Type: EventTrade, Market: "ETH-USD"})
	expectNothing(t, client)

	hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})
	if event := receive(t, client); event.Market != "BTC-USD" {
		t.Fatalf("market = %q", event.Market)
	}

	// Watching again replaces rather than adds.
	client.Watch([]string{"SOL-USD"})
	hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})
	expectNothing(t, client)
}

// The property everything else rests on: one client that has stopped reading is
// dropped, and Publish returns rather than waiting for it.
func TestASlowClientIsDroppedRatherThanWaitedFor(t *testing.T) {
	hub := NewHub()
	stalled := hub.Subscribe()
	healthy := hub.Subscribe()
	defer healthy.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Comfortably more than the backlog, and never read by `stalled`.
		//
		// The healthy client is drained in lockstep rather than by a second
		// goroutine: a drainer racing a tight publish loop can itself fall
		// behind and be dropped, which makes the test flaky about the very
		// distinction it is drawing.
		for i := 0; i < backlog*3; i++ {
			hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})

			select {
			case <-healthy.Events():
			default:
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a client that stopped reading")
	}

	select {
	case <-stalled.Done():
	case <-time.After(time.Second):
		t.Fatal("a client that fell behind was not dropped")
	}

	if hub.Clients() != 1 {
		t.Errorf("Clients() = %d, want only the healthy one", hub.Clients())
	}

	select {
	case <-healthy.Done():
		t.Error("the client that kept up was dropped too")
	default:
	}
}

func TestCloseIsIdempotentAndDetaches(t *testing.T) {
	hub := NewHub()
	client := hub.Subscribe()

	client.Close()
	client.Close() // must not panic on a second close

	select {
	case <-client.Done():
	default:
		t.Fatal("Done did not close")
	}
	if hub.Clients() != 0 {
		t.Errorf("Clients() = %d after Close", hub.Clients())
	}

	// Publishing to nobody is not an error.
	hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})
}

// Closing a client while events are being published is the ordinary case — a
// browser tab goes away mid-trade — and must not be a send on a closed channel.
func TestClosingDuringPublishIsSafe(t *testing.T) {
	hub := NewHub()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		client := hub.Subscribe()

		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})
			}
		}()
		go func() {
			defer wg.Done()
			client.Close()
		}()
	}

	wg.Wait()
}

// Watch is called from the connection's read loop while Publish runs on the
// settlement worker, so the two must not race.
func TestWatchDuringPublishIsSafe(t *testing.T) {
	hub := NewHub()
	client := hub.Subscribe()
	defer client.Close()

	go func() {
		for i := 0; i < 500; i++ {
			client.Watch([]string{"BTC-USD", "ETH-USD"})
		}
	}()

	for i := 0; i < 500; i++ {
		hub.Publish(Event{Type: EventTrade, Market: "BTC-USD"})
		select {
		case <-client.Events():
		default:
		}
	}
}
