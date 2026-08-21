package postgres

import (
	"context"
	"embed"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/portfolio/pf-commerce/apps/api/internal/cart"
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

func (s *Store) Get(ctx context.Context, buyerSub, orgID string) (cart.Cart, error) {
	rows, err := s.pool.Query(ctx, `SELECT product_id, qty, updated_at FROM cart_items WHERE buyer_sub=$1 AND org_id=$2`, buyerSub, orgID)
	if err != nil {
		return cart.Cart{}, err
	}
	defer rows.Close()
	c := cart.Cart{BuyerSub: buyerSub, OrgID: orgID, Items: []cart.Item{}}
	for rows.Next() {
		var it cart.Item
		if err := rows.Scan(&it.ProductID, &it.Qty, &it.UpdatedAt); err != nil {
			return cart.Cart{}, err
		}
		c.Items = append(c.Items, it)
	}
	return c, rows.Err()
}

func (s *Store) Replace(ctx context.Context, c cart.Cart) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE buyer_sub=$1 AND org_id=$2`, c.BuyerSub, c.OrgID); err != nil {
		return err
	}
	for _, it := range c.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO cart_items (buyer_sub, org_id, product_id, qty, updated_at) VALUES ($1,$2,$3,$4,$5)`,
			c.BuyerSub, c.OrgID, it.ProductID, it.Qty, it.UpdatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Clear(ctx context.Context, buyerSub, orgID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM cart_items WHERE buyer_sub=$1 AND org_id=$2`, buyerSub, orgID)
	return err
}
