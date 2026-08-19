package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/packages/id"
)

type Store struct {
	mu     sync.Mutex
	orders map[string]order.Order
	events map[string][]order.RecordedEvent
}

func New() *Store {
	return &Store{orders: map[string]order.Order{}, events: map[string][]order.RecordedEvent{}}
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Append(_ context.Context, streamID string, expectedVersion int, events []order.NewEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.events[streamID]
	if len(cur) != expectedVersion {
		return order.ErrConflict
	}
	if len(events) == 0 {
		return nil
	}
	for i, e := range events {
		raw, err := json.Marshal(e.Data)
		if err != nil {
			return err
		}
		if e.Data == nil {
			raw = []byte("{}")
		}
		rec := order.RecordedEvent{
			ID: id.New(), StreamID: streamID, Version: expectedVersion + i + 1,
			Type: e.Type, Time: e.Time, Data: raw,
		}
		if rec.Type == order.EventOrderCreated {
			var d order.OrderCreatedData
			if err := json.Unmarshal(raw, &d); err == nil {
				for _, existing := range s.orders {
					if existing.BuyerSub == d.BuyerSub && existing.IdempotencyKey == d.IdempotencyKey {
						return order.ErrConflict
					}
				}
			}
		}
		cur = append(cur, rec)
	}
	s.events[streamID] = cur
	o, err := order.Fold(cur)
	if err != nil {
		return err
	}
	s.orders[streamID] = cloneOrder(o)
	return nil
}

func (s *Store) Load(_ context.Context, streamID string) ([]order.RecordedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.events[streamID]
	if len(src) == 0 {
		if _, ok := s.orders[streamID]; !ok {
			return nil, order.ErrNotFound
		}
	}
	out := make([]order.RecordedEvent, len(src))
	copy(out, src)
	return out, nil
}

func (s *Store) LoadAll(context.Context) ([]order.RecordedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []order.RecordedEvent
	for _, evs := range s.events {
		out = append(out, evs...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StreamID == out[j].StreamID {
			return out[i].Version < out[j].Version
		}
		return out[i].StreamID < out[j].StreamID
	})
	return out, nil
}

func (s *Store) RebuildProjections(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders = map[string]order.Order{}
	for stream, evs := range s.events {
		o, err := order.Fold(evs)
		if err != nil {
			return err
		}
		s.orders[stream] = cloneOrder(o)
	}
	return nil
}

func (s *Store) Create(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.orders {
		if existing.BuyerSub == o.BuyerSub && existing.IdempotencyKey == o.IdempotencyKey {
			return order.ErrConflict
		}
	}
	s.orders[o.ID] = cloneOrder(o)
	return nil
}

func (s *Store) Get(_ context.Context, id string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return cloneOrder(o), nil
}

func (s *Store) GetByIdempotency(_ context.Context, buyerSub, key string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.BuyerSub == buyerSub && o.IdempotencyKey == key {
			return cloneOrder(o), nil
		}
	}
	return order.Order{}, order.ErrNotFound
}

func (s *Store) ListByBuyer(_ context.Context, buyerSub string) ([]order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []order.Order
	for _, o := range s.orders {
		if o.BuyerSub == buyerSub {
			out = append(out, cloneOrder(o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Update(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[o.ID]; !ok {
		return order.ErrNotFound
	}
	s.orders[o.ID] = cloneOrder(o)
	return nil
}

func cloneOrder(o order.Order) order.Order {
	o.Lines = append([]order.Line{}, o.Lines...)
	return o
}
