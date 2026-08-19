package postgres

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/packages/id"
	"github.com/portfolio/pf-commerce/packages/money"
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

func (s *Store) Create(ctx context.Context, o order.Order) error {
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
	for _, ln := range o.Lines {
		if _, err := tx.Exec(ctx, `INSERT INTO commerce_order_lines
			(order_id, product_id, sku, name, qty, unit_price_minor, currency)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			o.ID, ln.ProductID, ln.SKU, ln.Name, ln.Qty, ln.UnitPriceMinor, ln.Currency); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Get(ctx context.Context, id string) (order.Order, error) {
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

func (s *Store) GetByIdempotency(ctx context.Context, buyerSub, key string) (order.Order, error) {
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

func (s *Store) ListByBuyer(ctx context.Context, buyerSub string) ([]order.Order, error) {
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

func (s *Store) Update(ctx context.Context, o order.Order) error {
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

func (s *Store) Append(ctx context.Context, streamID string, expectedVersion int, events []order.NewEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM commerce_order_events WHERE stream_id=$1`, streamID).Scan(&current)
	if err != nil {
		return err
	}
	if current != expectedVersion {
		return order.ErrConflict
	}
	for i, e := range events {
		raw, err := json.Marshal(e.Data)
		if err != nil {
			return err
		}
		if e.Data == nil {
			raw = []byte("{}")
		}
		_, err = tx.Exec(ctx, `INSERT INTO commerce_order_events
			(stream_id, version, event_id, event_type, occurred_at, payload)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			streamID, expectedVersion+i+1, id.New(), string(e.Type), e.Time, raw)
		if isUnique(err) {
			return order.ErrConflict
		}
		if err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT stream_id, version, event_id, event_type, occurred_at, payload
		FROM commerce_order_events WHERE stream_id=$1 ORDER BY version`, streamID)
	if err != nil {
		return err
	}
	evs, err := scanEvents(rows)
	if err != nil {
		return err
	}
	o, err := order.Fold(evs)
	if err != nil {
		return err
	}
	if err := upsertProjection(ctx, tx, o); err != nil {
		if isUnique(err) {
			return order.ErrConflict
		}
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Load(ctx context.Context, streamID string) ([]order.RecordedEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT stream_id, version, event_id, event_type, occurred_at, payload
		FROM commerce_order_events WHERE stream_id=$1 ORDER BY version`, streamID)
	if err != nil {
		return nil, err
	}
	evs, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 {
		return nil, order.ErrNotFound
	}
	return evs, nil
}

func (s *Store) LoadAll(ctx context.Context) ([]order.RecordedEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT stream_id, version, event_id, event_type, occurred_at, payload
		FROM commerce_order_events ORDER BY stream_id, version`)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

func (s *Store) RebuildProjections(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM commerce_order_lines`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM commerce_orders`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT stream_id, version, event_id, event_type, occurred_at, payload
		FROM commerce_order_events ORDER BY stream_id, version`)
	if err != nil {
		return err
	}
	evs, err := scanEvents(rows)
	if err != nil {
		return err
	}
	byStream := map[string][]order.RecordedEvent{}
	for _, e := range evs {
		byStream[e.StreamID] = append(byStream[e.StreamID], e)
	}
	for _, stream := range byStream {
		o, err := order.Fold(stream)
		if err != nil {
			return err
		}
		if err := upsertProjection(ctx, tx, o); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func scanEvents(rows pgx.Rows) ([]order.RecordedEvent, error) {
	defer rows.Close()
	var out []order.RecordedEvent
	for rows.Next() {
		var e order.RecordedEvent
		var typ string
		if err := rows.Scan(&e.StreamID, &e.Version, &e.ID, &typ, &e.Time, &e.Data); err != nil {
			return nil, err
		}
		e.Type = order.EventType(typ)
		out = append(out, e)
	}
	return out, rows.Err()
}

func upsertProjection(ctx context.Context, tx pgx.Tx, o order.Order) error {
	_, err := tx.Exec(ctx, `INSERT INTO commerce_orders
		(id, buyer_sub, status, cancel_reason, amount_minor, currency, idempotency_key, payment_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			status=EXCLUDED.status, cancel_reason=EXCLUDED.cancel_reason, payment_id=EXCLUDED.payment_id, updated_at=EXCLUDED.updated_at`,
		o.ID, o.BuyerSub, o.Status, o.CancelReason, o.Amount.Minor, o.Amount.Currency, o.IdempotencyKey, o.PaymentID, o.CreatedAt, o.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM commerce_order_lines WHERE order_id=$1`, o.ID); err != nil {
		return err
	}
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
