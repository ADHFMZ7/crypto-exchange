package orderbook

import (
	"container/heap"
	"testing"
)

func TestQueueIsFIFO(t *testing.T) {
	var q Queue

	if q.Len() != 0 {
		t.Fatalf("fresh queue has length %d, want 0", q.Len())
	}

	for _, id := range []OrderID{1, 2, 3} {
		q.Enqueue(&Order{ID: id})
	}

	if q.Len() != 3 {
		t.Fatalf("length = %d, want 3", q.Len())
	}

	// Time priority at a price level is the queue's only job: first in, first
	// filled.
	for _, want := range []OrderID{1, 2, 3} {
		peeked, ok := q.Peek()
		if !ok {
			t.Fatalf("Peek reported empty with %d orders left", q.Len())
		}
		if peeked.ID != want {
			t.Fatalf("Peek returned order %d, want %d", peeked.ID, want)
		}

		dequeued, ok := q.Dequeue()
		if !ok {
			t.Fatal("Dequeue reported empty on a non-empty queue")
		}
		if dequeued.ID != want {
			t.Fatalf("Dequeue returned order %d, want %d", dequeued.ID, want)
		}
	}

	if q.Len() != 0 {
		t.Fatalf("length = %d after draining, want 0", q.Len())
	}
}

// Peek and Dequeue report emptiness through their second return value and hand
// back a zero Order rather than nil, so a caller that ignores `ok` gets a
// harmless empty order instead of a nil dereference.
func TestQueueEmptyBehaviour(t *testing.T) {
	var q Queue

	peeked, ok := q.Peek()
	if ok {
		t.Fatal("Peek on an empty queue reported ok")
	}
	if peeked == nil {
		t.Fatal("Peek returned nil; callers dereference this value")
	}

	dequeued, ok := q.Dequeue()
	if ok {
		t.Fatal("Dequeue on an empty queue reported ok")
	}
	if dequeued == nil {
		t.Fatal("Dequeue returned nil; callers dereference this value")
	}
}

func TestQueueHandsBackTheSamePointer(t *testing.T) {
	var q Queue
	order := &Order{ID: 1, Shares: 100}
	q.Enqueue(order)

	peeked, _ := q.Peek()
	// Matching mutates the resting order's Shares through this pointer, so a
	// copy would silently discard every partial fill.
	peeked.Shares = 40

	if order.Shares != 40 {
		t.Fatalf("mutation through Peek did not reach the queued order: %d", order.Shares)
	}
}

func TestMinHeapYieldsLowestPrice(t *testing.T) {
	h := &MinHeap{}
	heap.Init(h)

	if _, ok := h.Peek(); ok {
		t.Fatal("empty heap reported a price")
	}

	for _, p := range []Price{2500, 2300, 2400, 2600} {
		heap.Push(h, p)
	}

	// The best ask is the cheapest one.
	for _, want := range []Price{2300, 2400, 2500, 2600} {
		got, ok := h.Peek()
		if !ok {
			t.Fatal("Peek reported empty mid-drain")
		}
		if got != want {
			t.Fatalf("Peek = %d, want %d", got, want)
		}
		if popped := heap.Pop(h).(Price); popped != want {
			t.Fatalf("Pop = %d, want %d", popped, want)
		}
	}

	if _, ok := h.Peek(); ok {
		t.Fatal("drained heap still reports a price")
	}
}

func TestMaxHeapYieldsHighestPrice(t *testing.T) {
	h := &MaxHeap{}
	heap.Init(h)

	if _, ok := h.Peek(); ok {
		t.Fatal("empty heap reported a price")
	}

	for _, p := range []Price{2500, 2300, 2400, 2600} {
		heap.Push(h, p)
	}

	// The best bid is the most generous one. This ordering comes from MaxHeap
	// shadowing PriceHeap.Less — if that promotion ever breaks, the bid side
	// silently starts matching at the worst price instead of the best.
	for _, want := range []Price{2600, 2500, 2400, 2300} {
		got, ok := h.Peek()
		if !ok {
			t.Fatal("Peek reported empty mid-drain")
		}
		if got != want {
			t.Fatalf("Peek = %d, want %d", got, want)
		}
		if popped := heap.Pop(h).(Price); popped != want {
			t.Fatalf("Pop = %d, want %d", popped, want)
		}
	}
}

func TestHeapsTolerateDuplicatePrices(t *testing.T) {
	h := &MinHeap{}
	heap.Init(h)

	for _, p := range []Price{2400, 2400, 2300} {
		heap.Push(h, p)
	}

	for _, want := range []Price{2300, 2400, 2400} {
		if got := heap.Pop(h).(Price); got != want {
			t.Fatalf("Pop = %d, want %d", got, want)
		}
	}
}
