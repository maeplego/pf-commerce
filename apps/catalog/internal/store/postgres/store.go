package postgres

import (
	"context"
	"embed"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
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

func (s *Store) Create(ctx context.Context, p catalog.Product) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO catalog_products
		(id, org_id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		p.ID, p.OrgID, p.SKU, p.Name, p.Description, p.Price.Minor, p.Price.Currency, p.ImageURL, p.Active, p.CreatedAt, p.UpdatedAt)
	if isUnique(err) {
		return catalog.ErrConflict
	}
	return err
}

func scanProduct(row pgx.Row) (catalog.Product, error) {
	var p catalog.Product
	var minor int64
	var currency string
	err := row.Scan(&p.ID, &p.OrgID, &p.SKU, &p.Name, &p.Description, &minor, &currency, &p.ImageURL, &p.Active, &p.CreatedAt, &p.UpdatedAt)
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

const productCols = `id, org_id, sku, name, description, price_minor, currency, image_url, active, created_at, updated_at`

func (s *Store) Get(ctx context.Context, id string) (catalog.Product, error) {
	return scanProduct(s.pool.QueryRow(ctx, `SELECT `+productCols+` FROM catalog_products WHERE id=$1`, id))
}

func (s *Store) GetBySKU(ctx context.Context, sku string) (catalog.Product, error) {
	return scanProduct(s.pool.QueryRow(ctx, `SELECT `+productCols+` FROM catalog_products WHERE sku=$1`, sku))
}

func (s *Store) List(ctx context.Context, orgID string) ([]catalog.Product, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+productCols+` FROM catalog_products WHERE org_id=$1 ORDER BY sku`, orgID)
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

func (s *Store) AddReview(ctx context.Context, r catalog.Review) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO catalog_reviews (id, product_id, author, body, created_at)
		VALUES ($1,$2,$3,$4,$5)`, r.ID, r.ProductID, r.Author, r.Body, r.CreatedAt)
	return err
}

func (s *Store) ListReviews(ctx context.Context, productIDs []string) ([]catalog.Review, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id, product_id, author, body, created_at
		FROM catalog_reviews WHERE product_id = ANY($1) ORDER BY created_at`, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []catalog.Review
	for rows.Next() {
		var r catalog.Review
		if err := rows.Scan(&r.ID, &r.ProductID, &r.Author, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
