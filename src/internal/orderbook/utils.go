package orderbook

type PriceHeap []Price

type MinHeap struct{ PriceHeap }
type MaxHeap struct{ PriceHeap }

func (h PriceHeap) Len() int           { return len(h) }
func (h PriceHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h PriceHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *PriceHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(Price))
}

func (h *PriceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *PriceHeap) Peek() (Price, bool) {
	if h.Len() == 0 {
		return 0, false
	}
	return (*h)[0], true
}

func (h MaxHeap) Less(i, j int) bool { return h.PriceHeap[i] > h.PriceHeap[j] }

type Queue struct {
	Data []*Order
}

func (q *Queue) Peek() (*Order, bool) {

	if len(q.Data) == 0 {
		return &Order{}, false
	}

	return q.Data[0], true
}

func (q *Queue) Enqueue(order *Order) {
	q.Data = append(q.Data, order)
}

func (q *Queue) Dequeue() (*Order, bool) {

	if len(q.Data) == 0 {
		return &Order{}, false
	}

	order := q.Data[0]
	q.Data = q.Data[1:]

	return order, true
}

func (q *Queue) Len() int {
	return len(q.Data)
}
