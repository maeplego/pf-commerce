package payment

import (
	"context"
	"sync"
)

type Memory struct {
	mu   sync.Mutex
	byKey map[string]Charge
}

func NewMemory() *Memory {
	return &Memory{byKey: map[string]Charge{}}
}

func (m *Memory) Ping(context.Context) error { return nil }

func (m *Memory) GetByKey(_ context.Context, key string) (Charge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.byKey[key]
	if !ok {
		return Charge{}, ErrNotFound
	}
	return ch, nil
}

func (m *Memory) Insert(_ context.Context, ch Charge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byKey[ch.IdempotencyKey]; ok {
		return nil
	}
	m.byKey[ch.IdempotencyKey] = ch
	return nil
}
