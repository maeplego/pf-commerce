package postgres

import (
	"context"
	"embed"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/portfolio/pf-commerce/api/internal/cart"
	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
	"github.com/portfolio/pf-commerce/api/internal/money"
	"github.com/portfolio/pf-commerce/api/internal/order"
)

//go:embed schema.sql
var schemaFS embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) migrate(ctx context.Context) error {
	raw, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	for _, stmt := range splitSQL(string(raw)) {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func splitSQL(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ";") {
		s := strings.TrimSpace(part)
		if s == "" || strings.HasPrefix(s, "--") && !strings.Contains(s, "CREATE ") {
			continue
		}
		out = append(out, s)
	}
	return out
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func (s *Store) Create(ctx context.Context, p catalog.Product) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO catalog_products
		(id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		p.ID, p.SKU, p.Name, p.Description, p.Price.Minor, p.Price.Currency, p.ImageURL, p.Active, p.CreatedAt, p.UpdatedAt)
	if isUnique(err) {
		return catalog.ErrConflict
	}
	return err
}

func scanProduct(row pgx.Row) (catalog.Product, error) {
	var p catalog.Product
	var minor int64
	var currency string
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.Description, &minor, &currency, &p.ImageURL, &p.Active, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return catalog.Product{}, catalog.ErrNotFound
	}
	if err != nil {
		return catalog.Product{}, err
	}
	amt, err := money.New(minor, currency)
	if err != nil {
		return catalog.Product{}, err
	}
	p.Price = amt
	return p, nil
}

func (s *Store) Get(ctx context.Context, id string) (catalog.Product, error) {
	return scanProduct(s.pool.QueryRow(ctx, `SELECT id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at
		FROM catalog_products WHERE id=$1`, id))
}

func (s *Store) GetBySKU(ctx context.Context, sku string) (catalog.Product, error) {
	return scanProduct(s.pool.QueryRow(ctx, `SELECT id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at
		FROM catalog_products WHERE sku=$1`, sku))
}

func (s *Store) List(ctx context.Context) ([]catalog.Product, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at
		FROM catalog_products ORDER BY sku`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateSite(ctx context.Context, site inventory.Site) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO inventory_sites (id, code, name) VALUES ($1,$2,$3)`, site.ID, site.Code, site.Name)
	if isUnique(err) {
		return inventory.ErrConflict
	}
	return err
}

func (s *Store) GetSiteByCode(ctx context.Context, code string) (inventory.Site, error) {
	var site inventory.Site
	err := s.pool.QueryRow(ctx, `SELECT id, code, name FROM inventory_sites WHERE code=$1`, code).Scan(&site.ID, &site.Code, &site.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return inventory.Site{}, inventory.ErrNotFound
	}
	return site, err
}

func scanBalance(row pgx.Row) (inventory.Balance, error) {
	var b inventory.Balance
	err := row.Scan(&b.SiteID, &b.ProductID, &b.Qty, &b.ReservedQty, &b.Version, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return inventory.Balance{}, inventory.ErrNotFound
	}
	return b, err
}

func (s *Store) GetBalance(ctx context.Context, siteID, productID string) (inventory.Balance, error) {
	return scanBalance(s.pool.QueryRow(ctx, `SELECT site_id, product_id, qty, reserved_qty, version, updated_at
		FROM inventory_stock_balances WHERE site_id=$1 AND product_id=$2`, siteID, productID))
}

func (s *Store) EnsureBalance(ctx context.Context, siteID, productID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO inventory_stock_balances
		(site_id, product_id, qty, reserved_qty, version, updated_at)
		VALUES ($1,$2,0,0,1,$3)
		ON CONFLICT (site_id, product_id) DO NOTHING`, siteID, productID, now)
	return err
}

func (s *Store) AdjustOnHand(ctx context.Context, siteID, productID string, deltaQty int, now time.Time) (inventory.Balance, error) {
	b, err := scanBalance(s.pool.QueryRow(ctx, `UPDATE inventory_stock_balances
		SET qty = qty + $3, version = version + 1, updated_at = $4
		WHERE site_id=$1 AND product_id=$2 AND qty + $3 >= 0 AND reserved_qty <= qty + $3
		RETURNING site_id, product_id, qty, reserved_qty, version, updated_at`,
		siteID, productID, deltaQty, now))
	if errors.Is(err, inventory.ErrNotFound) {
		return inventory.Balance{}, inventory.ErrInvalid
	}
	return b, err
}

func (s *Store) TryReserve(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	b, err := scanBalance(s.pool.QueryRow(ctx, `UPDATE inventory_stock_balances
		SET reserved_qty = reserved_qty + $3, version = version + 1, updated_at = $4
		WHERE site_id=$1 AND product_id=$2 AND (qty - reserved_qty) >= $3
		RETURNING site_id, product_id, qty, reserved_qty, version, updated_at`,
		siteID, productID, qty, now))
	if errors.Is(err, inventory.ErrNotFound) {
		return inventory.Balance{}, inventory.ErrShortage
	}
	return b, err
}

func (s *Store) ReleaseReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	b, err := scanBalance(s.pool.QueryRow(ctx, `UPDATE inventory_stock_balances
		SET reserved_qty = reserved_qty - $3, version = version + 1, updated_at = $4
		WHERE site_id=$1 AND product_id=$2 AND reserved_qty >= $3
		RETURNING site_id, product_id, qty, reserved_qty, version, updated_at`,
		siteID, productID, qty, now))
	if errors.Is(err, inventory.ErrNotFound) {
		return inventory.Balance{}, inventory.ErrInvalid
	}
	return b, err
}

func (s *Store) ConsumeReserved(ctx context.Context, siteID, productID string, qty int, now time.Time) (inventory.Balance, error) {
	b, err := scanBalance(s.pool.QueryRow(ctx, `UPDATE inventory_stock_balances
		SET qty = qty - $3, reserved_qty = reserved_qty - $3, version = version + 1, updated_at = $4
		WHERE site_id=$1 AND product_id=$2 AND qty >= $3 AND reserved_qty >= $3
		RETURNING site_id, product_id, qty, reserved_qty, version, updated_at`,
		siteID, productID, qty, now))
	if errors.Is(err, inventory.ErrNotFound) {
		return inventory.Balance{}, inventory.ErrInvalid
	}
	return b, err
}

func (s *Store) InsertMovement(ctx context.Context, m inventory.Movement) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO inventory_stock_movements
		(id, site_id, product_id, type, qty, reason, actor_id, reservation_id, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		m.ID, m.SiteID, m.ProductID, m.Type, m.Qty, m.Reason, m.ActorID, nullIfEmpty(m.ReservationID), m.OccurredAt)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) InsertReservation(ctx context.Context, r inventory.Reservation) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO inventory_reservations
		(id, site_id, product_id, qty, order_id, status, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		r.ID, r.SiteID, r.ProductID, r.Qty, r.OrderID, r.Status, r.ExpiresAt, r.CreatedAt)
	return err
}

func (s *Store) GetReservation(ctx context.Context, id string) (inventory.Reservation, error) {
	var r inventory.Reservation
	err := s.pool.QueryRow(ctx, `SELECT id, site_id, product_id, qty, order_id, status, expires_at, created_at
		FROM inventory_reservations WHERE id=$1`, id).Scan(
		&r.ID, &r.SiteID, &r.ProductID, &r.Qty, &r.OrderID, &r.Status, &r.ExpiresAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return inventory.Reservation{}, inventory.ErrNotFound
	}
	return r, err
}

func (s *Store) ListHeldByOrder(ctx context.Context, orderID string) ([]inventory.Reservation, error) {
	return s.listReservations(ctx, `SELECT id, site_id, product_id, qty, order_id, status, expires_at, created_at
		FROM inventory_reservations WHERE order_id=$1 AND status='held'`, orderID)
}

func (s *Store) ListExpiredHeld(ctx context.Context, now time.Time) ([]inventory.Reservation, error) {
	return s.listReservations(ctx, `SELECT id, site_id, product_id, qty, order_id, status, expires_at, created_at
		FROM inventory_reservations WHERE status='held' AND expires_at <= $1`, now)
}

func (s *Store) listReservations(ctx context.Context, q string, args ...any) ([]inventory.Reservation, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inventory.Reservation
	for rows.Next() {
		var r inventory.Reservation
		if err := rows.Scan(&r.ID, &r.SiteID, &r.ProductID, &r.Qty, &r.OrderID, &r.Status, &r.ExpiresAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateReservationStatus(ctx context.Context, id string, from, to inventory.ReservationStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE inventory_reservations SET status=$3 WHERE id=$1 AND status=$2`, id, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return inventory.ErrConflict
	}
	return nil
}

func (s *Store) CartGet(ctx context.Context, buyerSub string) (cart.Cart, error) {
	rows, err := s.pool.Query(ctx, `SELECT product_id, qty, updated_at FROM cart_items WHERE buyer_sub=$1`, buyerSub)
	if err != nil {
		return cart.Cart{}, err
	}
	defer rows.Close()
	c := cart.Cart{BuyerSub: buyerSub, Items: []cart.Item{}}
	for rows.Next() {
		var it cart.Item
		if err := rows.Scan(&it.ProductID, &it.Qty, &it.UpdatedAt); err != nil {
			return cart.Cart{}, err
		}
		c.Items = append(c.Items, it)
	}
	return c, rows.Err()
}

func (s *Store) CartReplace(ctx context.Context, c cart.Cart) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE buyer_sub=$1`, c.BuyerSub); err != nil {
		return err
	}
	for _, it := range c.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO cart_items (buyer_sub, product_id, qty, updated_at) VALUES ($1,$2,$3,$4)`,
			c.BuyerSub, it.ProductID, it.Qty, it.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) CartClear(ctx context.Context, buyerSub string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM cart_items WHERE buyer_sub=$1`, buyerSub)
	return err
}

func (s *Store) OrderCreate(ctx context.Context, o order.Order) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO commerce_orders
		(id, buyer_sub, status, cancel_reason, amount_minor, currency, idempotency_key, payment_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		o.ID, o.BuyerSub, o.Status, o.CancelReason, o.Amount.Minor, o.Amount.Currency, o.IdempotencyKey, o.PaymentID, o.CreatedAt, o.UpdatedAt)
	if isUnique(err) {
		return order.ErrConflict
	}
	if err != nil {
		return err
	}
	if err := insertLines(ctx, tx, o); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertLines(ctx context.Context, tx pgx.Tx, o order.Order) error {
	for _, ln := range o.Lines {
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_order_lines
			(order_id, product_id, sku, name, qty, unit_price_minor, currency)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			o.ID, ln.ProductID, ln.SKU, ln.Name, ln.Qty, ln.UnitPriceMinor, ln.Currency); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) OrderGet(ctx context.Context, id string) (order.Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `SELECT id, buyer_sub, status, cancel_reason, amount_minor, currency, idempotency_key, payment_id, created_at, updated_at
		FROM commerce_orders WHERE id=$1`, id))
	if err != nil {
		return order.Order{}, err
	}
	lines, err := s.loadLines(ctx, o.ID)
	if err != nil {
		return order.Order{}, err
	}
	o.Lines = lines
	return o, nil
}

func (s *Store) OrderGetByIdempotency(ctx context.Context, buyerSub, key string) (order.Order, error) {
	o, err := scanOrder(s.pool.QueryRow(ctx, `SELECT id, buyer_sub, status, cancel_reason, amount_minor, currency, idempotency_key, payment_id, created_at, updated_at
		FROM commerce_orders WHERE buyer_sub=$1 AND idempotency_key=$2`, buyerSub, key))
	if err != nil {
		return order.Order{}, err
	}
	lines, err := s.loadLines(ctx, o.ID)
	if err != nil {
		return order.Order{}, err
	}
	o.Lines = lines
	return o, nil
}

func (s *Store) OrderListByBuyer(ctx context.Context, buyerSub string) ([]order.Order, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, buyer_sub, status, cancel_reason, amount_minor, currency, idempotency_key, payment_id, created_at, updated_at
		FROM commerce_orders WHERE buyer_sub=$1 ORDER BY created_at DESC`, buyerSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []order.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		lines, err := s.loadLines(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Lines = lines
	}
	return out, nil
}

func (s *Store) OrderUpdate(ctx context.Context, o order.Order) error {
	tag, err := s.pool.Exec(ctx, `UPDATE commerce_orders SET status=$2, cancel_reason=$3, payment_id=$4, updated_at=$5 WHERE id=$1`,
		o.ID, o.Status, o.CancelReason, o.PaymentID, o.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return order.ErrNotFound
	}
	return nil
}

func (s *Store) loadLines(ctx context.Context, orderID string) ([]order.Line, error) {
	rows, err := s.pool.Query(ctx, `SELECT product_id, sku, name, qty, unit_price_minor, currency
		FROM commerce_order_lines WHERE order_id=$1`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []order.Line
	for rows.Next() {
		var ln order.Line
		if err := rows.Scan(&ln.ProductID, &ln.SKU, &ln.Name, &ln.Qty, &ln.UnitPriceMinor, &ln.Currency); err != nil {
			return nil, err
		}
		lines = append(lines, ln)
	}
	return lines, rows.Err()
}

func scanOrder(row pgx.Row) (order.Order, error) {
	var o order.Order
	var minor int64
	var currency string
	err := row.Scan(&o.ID, &o.BuyerSub, &o.Status, &o.CancelReason, &minor, &currency, &o.IdempotencyKey, &o.PaymentID, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Order{}, order.ErrNotFound
	}
	if err != nil {
		return order.Order{}, err
	}
	amt, err := money.New(minor, currency)
	if err != nil {
		return order.Order{}, err
	}
	o.Amount = amt
	return o, nil
}

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
