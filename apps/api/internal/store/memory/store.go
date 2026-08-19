package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/portfolio/pf-commerce/api/internal/cart"
	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
	"github.com/portfolio/pf-commerce/api/internal/order"
)

type Store struct {
	mu            sync.Mutex
	products      map[string]catalog.Product
	productsBySKU map[string]string
	sites         map[string]inventory.Site
	sitesByCode   map[string]string
	balances      map[string]inventory.Balance
	movements     []inventory.Movement
	reservations  map[string]inventory.Reservation
	carts         map[string]cart.Cart
	orders        map[string]order.Order
}

func New() *Store {
	return &Store{
		products:      map[string]catalog.Product{},
		productsBySKU: map[string]string{},
		sites:         map[string]inventory.Site{},
		sitesByCode:   map[string]string{},
		balances:      map[string]inventory.Balance{},
		reservations:  map[string]inventory.Reservation{},
		carts:         map[string]cart.Cart{},
		orders:        map[string]order.Order{},
	}
}

func balKey(siteID, productID string) string {
	return siteID + "\x00" + productID
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Create(_ context.Context, p catalog.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.productsBySKU[p.SKU]; ok {
		return catalog.ErrConflict
	}
	cp := p
	s.products[p.ID] = cp
	s.productsBySKU[p.SKU] = p.ID
	return nil
}

func (s *Store) Get(_ context.Context, id string) (catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.products[id]
	if !ok {
		return catalog.Product{}, catalog.ErrNotFound
	}
	return p, nil
}

func (s *Store) GetBySKU(_ context.Context, sku string) (catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.productsBySKU[sku]
	if !ok {
		return catalog.Product{}, catalog.ErrNotFound
	}
	return s.products[id], nil
}

func (s *Store) List(context.Context) ([]catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]catalog.Product, 0, len(s.products))
	for _, p := range s.products {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out, nil
}

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

func (s *Store) TryReserve(_ context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := balKey(siteID, productID)
	b := s.balances[k]
	if b.Available() < qty {
		return inventory.Balance{}, inventory.ErrShortage
	}
	b.ReservedQty += qty
	b.Version++
	b.UpdatedAt = now
	s.balances[k] = b
	return b, nil
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

func (s *Store) InsertReservation(_ context.Context, r inventory.Reservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reservations[r.ID] = r
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

func (s *Store) CartGet(_ context.Context, buyerSub string) (cart.Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.carts[buyerSub]
	if !ok {
		return cart.Cart{BuyerSub: buyerSub, Items: []cart.Item{}}, nil
	}
	return c, nil
}

func (s *Store) CartReplace(_ context.Context, c cart.Cart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]cart.Item{}, c.Items...)
	s.carts[c.BuyerSub] = cart.Cart{BuyerSub: c.BuyerSub, Items: items}
	return nil
}

func (s *Store) CartClear(_ context.Context, buyerSub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.carts, buyerSub)
	return nil
}

func (s *Store) OrderCreate(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.orders {
		if existing.BuyerSub == o.BuyerSub && existing.IdempotencyKey == o.IdempotencyKey {
			return order.ErrConflict
		}
	}
	cp := o
	cp.Lines = append([]order.Line{}, o.Lines...)
	s.orders[o.ID] = cp
	return nil
}

func (s *Store) OrderGet(_ context.Context, id string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return cloneOrder(o), nil
}

func (s *Store) OrderGetByIdempotency(_ context.Context, buyerSub, key string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.BuyerSub == buyerSub && o.IdempotencyKey == key {
			return cloneOrder(o), nil
		}
	}
	return order.Order{}, order.ErrNotFound
}

func (s *Store) OrderListByBuyer(_ context.Context, buyerSub string) ([]order.Order, error) {
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

func (s *Store) OrderUpdate(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[o.ID]; !ok {
		return order.ErrNotFound
	}
	cp := o
	cp.Lines = append([]order.Line{}, o.Lines...)
	s.orders[o.ID] = cp
	return nil
}

func cloneOrder(o order.Order) order.Order {
	o.Lines = append([]order.Line{}, o.Lines...)
	return o
}

// Adapters so one Store satisfies module Repository interfaces without method-name clashes.

type Catalog struct{ *Store }

func (c Catalog) Create(ctx context.Context, p catalog.Product) error { return c.Store.Create(ctx, p) }
func (c Catalog) Get(ctx context.Context, id string) (catalog.Product, error) {
	return c.Store.Get(ctx, id)
}
func (c Catalog) GetBySKU(ctx context.Context, sku string) (catalog.Product, error) {
	return c.Store.GetBySKU(ctx, sku)
}
func (c Catalog) List(ctx context.Context) ([]catalog.Product, error) { return c.Store.List(ctx) }

type Inventory struct{ *Store }

func (i Inventory) CreateSite(ctx context.Context, site inventory.Site) error {
	return i.Store.CreateSite(ctx, site)
}
func (i Inventory) GetSiteByCode(ctx context.Context, code string) (inventory.Site, error) {
	return i.Store.GetSiteByCode(ctx, code)
}
func (i Inventory) GetBalance(ctx context.Context, siteID, productID string) (inventory.Balance, error) {
	return i.Store.GetBalance(ctx, siteID, productID)
}
func (i Inventory) EnsureBalance(ctx context.Context, siteID, productID string, now time.Time) error {
	return i.Store.EnsureBalance(ctx, siteID, productID, now)
}
func (i Inventory) AdjustOnHand(ctx context.Context, siteID, productID string, deltaQty int, now time.Time) (inventory.Balance, error) {
	return i.Store.AdjustOnHand(ctx, siteID, productID, deltaQty, now)
}
func (i Inventory) TryReserve(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	return i.Store.TryReserve(ctx, siteID, productID, qty, now)
}
func (i Inventory) ReleaseReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	return i.Store.ReleaseReserved(ctx, siteID, productID, qty, now)
}
func (i Inventory) ConsumeReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	return i.Store.ConsumeReserved(ctx, siteID, productID, qty, now)
}
func (i Inventory) InsertMovement(ctx context.Context, m inventory.Movement) error {
	return i.Store.InsertMovement(ctx, m)
}
func (i Inventory) InsertReservation(ctx context.Context, r inventory.Reservation) error {
	return i.Store.InsertReservation(ctx, r)
}
func (i Inventory) GetReservation(ctx context.Context, id string) (inventory.Reservation, error) {
	return i.Store.GetReservation(ctx, id)
}
func (i Inventory) ListHeldByOrder(ctx context.Context, orderID string) ([]inventory.Reservation, error) {
	return i.Store.ListHeldByOrder(ctx, orderID)
}
func (i Inventory) ListExpiredHeld(ctx context.Context, now time.Time) ([]inventory.Reservation, error) {
	return i.Store.ListExpiredHeld(ctx, now)
}
func (i Inventory) UpdateReservationStatus(ctx context.Context, id string, from, to inventory.ReservationStatus) error {
	return i.Store.UpdateReservationStatus(ctx, id, from, to)
}

type Cart struct{ *Store }

func (c Cart) Get(ctx context.Context, buyerSub string) (cart.Cart, error) {
	return c.Store.CartGet(ctx, buyerSub)
}
func (c Cart) Replace(ctx context.Context, x cart.Cart) error { return c.Store.CartReplace(ctx, x) }
func (c Cart) Clear(ctx context.Context, buyerSub string) error {
	return c.Store.CartClear(ctx, buyerSub)
}

type Orders struct{ *Store }

func (o Orders) Create(ctx context.Context, x order.Order) error { return o.Store.OrderCreate(ctx, x) }
func (o Orders) Get(ctx context.Context, id string) (order.Order, error) {
	return o.Store.OrderGet(ctx, id)
}
func (o Orders) GetByIdempotency(ctx context.Context, buyerSub, key string) (order.Order, error) {
	return o.Store.OrderGetByIdempotency(ctx, buyerSub, key)
}
func (o Orders) ListByBuyer(ctx context.Context, buyerSub string) ([]order.Order, error) {
	return o.Store.OrderListByBuyer(ctx, buyerSub)
}
func (o Orders) Update(ctx context.Context, x order.Order) error { return o.Store.OrderUpdate(ctx, x) }
