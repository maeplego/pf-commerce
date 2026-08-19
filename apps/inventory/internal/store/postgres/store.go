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

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/packages/id"
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

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

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
		if s == "" {
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

func (s *Store) ReserveHeld(ctx context.Context, r inventory.Reservation, actorID, reason string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `UPDATE inventory_stock_balances
		SET reserved_qty = reserved_qty + $3, version = version + 1, updated_at = $4
		WHERE site_id=$1 AND product_id=$2 AND (qty - reserved_qty) >= $3`,
		r.SiteID, r.ProductID, r.Qty, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return inventory.ErrShortage
	}
	_, err = tx.Exec(ctx, `INSERT INTO inventory_reservations
		(id, site_id, product_id, qty, order_id, status, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		r.ID, r.SiteID, r.ProductID, r.Qty, r.OrderID, r.Status, r.ExpiresAt, r.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO inventory_stock_movements
		(id, site_id, product_id, type, qty, reason, actor_id, reservation_id, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id.New(), r.SiteID, r.ProductID, inventory.MoveReserve, r.Qty, reason, actorID, r.ID, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
