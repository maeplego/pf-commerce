package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/packages/id"
)

type Store struct {
	mu           sync.Mutex
	sites        map[string]inventory.Site
	sitesByCode  map[string]string
	balances     map[string]inventory.Balance
	movements    []inventory.Movement
	reservations map[string]inventory.Reservation
}

func New() *Store {
	return &Store{
		sites:        map[string]inventory.Site{},
		sitesByCode:  map[string]string{},
		balances:     map[string]inventory.Balance{},
		reservations: map[string]inventory.Reservation{},
	}
}

func balKey(siteID, productID string) string { return siteID + "\x00" + productID }

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) CreateSite(_ context.Context, site inventory.Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sitesByCode[site.Code]; ok {
		return inventory.ErrConflict
	}
	s.sites[site.ID] = site
	s.sitesByCode[site.Code] = site.ID
	return nil
}

func (s *Store) GetSiteByCode(_ context.Context, code string) (inventory.Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.sitesByCode[code]
	if !ok {
		return inventory.Site{}, inventory.ErrNotFound
	}
	return s.sites[id], nil
}

func (s *Store) GetBalance(_ context.Context, siteID, productID string) (inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.balances[balKey(siteID, productID)]
	if !ok {
		return inventory.Balance{}, inventory.ErrNotFound
	}
	return b, nil
}

func (s *Store) EnsureBalance(_ context.Context, siteID, productID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := balKey(siteID, productID)
	if _, ok := s.balances[k]; ok {
		return nil
	}
	s.balances[k] = inventory.Balance{SiteID: siteID, ProductID: productID, Version: 1, UpdatedAt: now}
	return nil
}

func (s *Store) AdjustOnHand(_ context.Context, siteID, productID string, deltaQty int, now time.Time) (inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adjustLocked(siteID, productID, deltaQty, 0, now)
}

func (s *Store) ReleaseReserved(_ context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adjustLocked(siteID, productID, 0, -qty, now)
}

func (s *Store) ConsumeReserved(_ context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.adjustLocked(siteID, productID, -qty, -qty, now)
}

func (s *Store) adjustLocked(siteID, productID string, deltaQty, deltaReserved int, now time.Time) (inventory.Balance, error) {
	k := balKey(siteID, productID)
	b, ok := s.balances[k]
	if !ok {
		return inventory.Balance{}, inventory.ErrNotFound
	}
	nextQty := b.Qty + deltaQty
	nextRes := b.ReservedQty + deltaReserved
	if nextQty < 0 || nextRes < 0 || nextRes > nextQty {
		return inventory.Balance{}, inventory.ErrInvalid
	}
	b.Qty = nextQty
	b.ReservedQty = nextRes
	b.Version++
	b.UpdatedAt = now
	s.balances[k] = b
	return b, nil
}

func (s *Store) InsertMovement(_ context.Context, m inventory.Movement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.movements = append(s.movements, m)
	return nil
}

func (s *Store) GetReservation(_ context.Context, id string) (inventory.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[id]
	if !ok {
		return inventory.Reservation{}, inventory.ErrNotFound
	}
	return r, nil
}

func (s *Store) ListHeldByOrder(_ context.Context, orderID string) ([]inventory.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []inventory.Reservation
	for _, r := range s.reservations {
		if r.OrderID == orderID && r.Status == inventory.ResHeld {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListExpiredHeld(_ context.Context, now time.Time) ([]inventory.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []inventory.Reservation
	for _, r := range s.reservations {
		if r.Status == inventory.ResHeld && !r.ExpiresAt.After(now) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) UpdateReservationStatus(_ context.Context, id string, from, to inventory.ReservationStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.reservations[id]
	if !ok {
		return inventory.ErrNotFound
	}
	if r.Status != from {
		return inventory.ErrConflict
	}
	r.Status = to
	s.reservations[id] = r
	return nil
}

func (s *Store) ReserveHeld(_ context.Context, r inventory.Reservation, actorID, reason string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := balKey(r.SiteID, r.ProductID)
	b := s.balances[k]
	if b.Available() < r.Qty {
		return inventory.ErrShortage
	}
	b.ReservedQty += r.Qty
	b.Version++
	b.UpdatedAt = now
	s.balances[k] = b
	s.reservations[r.ID] = r
	s.movements = append(s.movements, inventory.Movement{
		ID:            id.New(),
		SiteID:        r.SiteID,
		ProductID:     r.ProductID,
		Type:          inventory.MoveReserve,
		Qty:           r.Qty,
		Reason:        reason,
		ActorID:       actorID,
		ReservationID: r.ID,
		OccurredAt:    now,
	})
	return nil
}

func (s *Store) ListBalances(_ context.Context, siteID, afterProductID string, limit int) ([]inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []inventory.Balance
	for _, b := range s.balances {
		if b.SiteID != siteID {
			continue
		}
		if afterProductID != "" && b.ProductID <= afterProductID {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProductID < out[j].ProductID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
