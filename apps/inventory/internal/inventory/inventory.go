package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/portfolio/pf-commerce/packages/id"
)

var (
	ErrNotFound = errors.New("inventory not found")
	ErrInvalid  = errors.New("invalid inventory")
	ErrShortage = errors.New("inventory shortage")
	ErrConflict = errors.New("inventory conflict")
	ErrExpired  = errors.New("reservation expired")
)

const DefaultSiteCode = "MAIN"

type Site struct {
	ID   string
	Code string
	Name string
}

type Balance struct {
	SiteID      string
	ProductID   string
	Qty         int
	ReservedQty int
	Version     int
	UpdatedAt   time.Time
}

func (b Balance) Available() int {
	a := b.Qty - b.ReservedQty
	if a < 0 {
		return 0
	}
	return a
}

type MovementType string

const (
	MoveInbound MovementType = "inbound"
	MoveReserve MovementType = "reserve"
	MoveRelease MovementType = "release"
	MoveConsume MovementType = "consume"
	MoveExpire  MovementType = "expire"
)

type Movement struct {
	ID            string
	SiteID        string
	ProductID     string
	Type          MovementType
	Qty           int
	Reason        string
	ActorID       string
	ReservationID string
	OccurredAt    time.Time
}

type ReservationStatus string

const (
	ResHeld     ReservationStatus = "held"
	ResConsumed ReservationStatus = "consumed"
	ResReleased ReservationStatus = "released"
	ResExpired  ReservationStatus = "expired"
)

type Reservation struct {
	ID        string
	SiteID    string
	ProductID string
	Qty       int
	OrderID   string
	Status    ReservationStatus
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Repository interface {
	CreateSite(ctx context.Context, site Site) error
	GetSiteByCode(ctx context.Context, code string) (Site, error)

	GetBalance(ctx context.Context, siteID, productID string) (Balance, error)
	EnsureBalance(ctx context.Context, siteID, productID string, now time.Time) error
	AdjustOnHand(ctx context.Context, siteID, productID string, deltaQty int, now time.Time) (Balance, error)
	ReleaseReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (Balance, error)
	ConsumeReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (Balance, error)

	InsertMovement(ctx context.Context, m Movement) error
	GetReservation(ctx context.Context, id string) (Reservation, error)
	ListHeldByOrder(ctx context.Context, orderID string) ([]Reservation, error)
	ListExpiredHeld(ctx context.Context, now time.Time) ([]Reservation, error)
	UpdateReservationStatus(ctx context.Context, id string, from, to ReservationStatus) error

	// ReserveHeld updates reserved qty, inserts the reservation row, and writes a movement in one transaction.
	ReserveHeld(ctx context.Context, r Reservation, actorID, reason string, now time.Time) error
}

type Service struct {
	repo Repository
	ttl  time.Duration
	now  func() time.Time
}

func NewService(repo Repository, ttl time.Duration, now func() time.Time) *Service {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, ttl: ttl, now: now}
}

func (s *Service) CreateSite(ctx context.Context, code, name string) (Site, error) {
	if code == "" || name == "" {
		return Site{}, ErrInvalid
	}
	if existing, err := s.repo.GetSiteByCode(ctx, code); err == nil {
		return existing, nil
	} else if err != ErrNotFound {
		return Site{}, err
	}
	site := Site{ID: id.New(), Code: code, Name: name}
	if err := s.repo.CreateSite(ctx, site); err != nil {
		return Site{}, err
	}
	return site, nil
}

func (s *Service) SiteByCode(ctx context.Context, code string) (Site, error) {
	return s.repo.GetSiteByCode(ctx, code)
}

func (s *Service) Available(ctx context.Context, siteID, productID string) (int, error) {
	b, err := s.repo.GetBalance(ctx, siteID, productID)
	if err == ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return b.Available(), nil
}

func (s *Service) Inbound(ctx context.Context, siteID, productID, actorID, reason string, qty int) (Balance, error) {
	if qty <= 0 {
		return Balance{}, ErrInvalid
	}
	if err := id.Parse(siteID); err != nil {
		return Balance{}, ErrInvalid
	}
	if err := id.Parse(productID); err != nil {
		return Balance{}, ErrInvalid
	}
	now := s.now()
	if err := s.repo.EnsureBalance(ctx, siteID, productID, now); err != nil {
		return Balance{}, err
	}
	b, err := s.repo.AdjustOnHand(ctx, siteID, productID, qty, now)
	if err != nil {
		return Balance{}, err
	}
	err = s.repo.InsertMovement(ctx, Movement{
		ID:         id.New(),
		SiteID:     siteID,
		ProductID:  productID,
		Type:       MoveInbound,
		Qty:        qty,
		Reason:     reason,
		ActorID:    actorID,
		OccurredAt: now,
	})
	return b, err
}

func (s *Service) Reserve(ctx context.Context, siteID, productID, orderID, actorID string, qty int) (Reservation, error) {
	if qty <= 0 || orderID == "" {
		return Reservation{}, ErrInvalid
	}
	if err := s.ExpireDue(ctx); err != nil {
		return Reservation{}, err
	}
	now := s.now()
	if err := s.repo.EnsureBalance(ctx, siteID, productID, now); err != nil {
		return Reservation{}, err
	}
	res := Reservation{
		ID:        id.New(),
		SiteID:    siteID,
		ProductID: productID,
		Qty:       qty,
		OrderID:   orderID,
		Status:    ResHeld,
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	}
	if err := s.repo.ReserveHeld(ctx, res, actorID, "checkout", now); err != nil {
		return Reservation{}, err
	}
	return res, nil
}

func (s *Service) ReleaseOrder(ctx context.Context, orderID, actorID, reason string) error {
	held, err := s.repo.ListHeldByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range held {
		if err := s.releaseOne(ctx, r, ResReleased, MoveRelease, actorID, reason, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ConsumeOrder(ctx context.Context, orderID, actorID string) error {
	held, err := s.repo.ListHeldByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range held {
		if !r.ExpiresAt.After(now) {
			return ErrExpired
		}
		if _, err := s.repo.ConsumeReserved(ctx, r.SiteID, r.ProductID, r.Qty, now); err != nil {
			return err
		}
		if err := s.repo.UpdateReservationStatus(ctx, r.ID, ResHeld, ResConsumed); err != nil {
			return err
		}
		if err := s.repo.InsertMovement(ctx, Movement{
			ID:            id.New(),
			SiteID:        r.SiteID,
			ProductID:     r.ProductID,
			Type:          MoveConsume,
			Qty:           r.Qty,
			Reason:        "paid",
			ActorID:       actorID,
			ReservationID: r.ID,
			OccurredAt:    now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ExpireDue(ctx context.Context) error {
	now := s.now()
	due, err := s.repo.ListExpiredHeld(ctx, now)
	if err != nil {
		return err
	}
	for _, r := range due {
		if err := s.releaseOne(ctx, r, ResExpired, MoveExpire, "system", "ttl", now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) releaseOne(ctx context.Context, r Reservation, to ReservationStatus, move MovementType, actorID, reason string, now time.Time) error {
	if _, err := s.repo.ReleaseReserved(ctx, r.SiteID, r.ProductID, r.Qty, now); err != nil {
		return err
	}
	if err := s.repo.UpdateReservationStatus(ctx, r.ID, ResHeld, to); err != nil {
		return err
	}
	return s.repo.InsertMovement(ctx, Movement{
		ID:            id.New(),
		SiteID:        r.SiteID,
		ProductID:     r.ProductID,
		Type:          move,
		Qty:           r.Qty,
		Reason:        reason,
		ActorID:       actorID,
		ReservationID: r.ID,
		OccurredAt:    now,
	})
}
