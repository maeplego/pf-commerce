package hub

import (
	"encoding/json"
	"sync"
)

type Event struct {
	ProductID    string `json:"productId"`
	Qty          int    `json:"qty"`
	ReservedQty  int    `json:"reservedQty"`
	AvailableQty int    `json:"availableQty"`
	Reason       string `json:"reason"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func New() *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}}
}

func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *Hub) Publish(ev Event) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- raw:
		default:
		}
	}
}
