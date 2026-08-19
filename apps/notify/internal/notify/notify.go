package notify

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid notification")

type Message struct {
	ID        string
	Type      string
	OrderID   string
	BuyerSub  string
	Payload   string
	CreatedAt time.Time
}

type Service struct {
	mu   sync.Mutex
	byID map[string]Message
	now  func() time.Time
}

func NewService(now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{byID: map[string]Message{}, now: now}
}

func (s *Service) Ping(context.Context) error { return nil }

func (s *Service) Deliver(_ context.Context, msg Message) (Message, bool, error) {
	if msg.ID == "" || msg.Type == "" || msg.OrderID == "" {
		return Message{}, false, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.byID[msg.ID]; ok {
		return existing, false, nil
	}
	msg.CreatedAt = s.now()
	s.byID[msg.ID] = msg
	return msg, true, nil
}

func (s *Service) List(_ context.Context) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, len(s.byID))
	for _, m := range s.byID {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
