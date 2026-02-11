package blueprint

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new Repository backed by the given connection pool.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &PostgresRepository{pool: pool}
}

// allColumns is the ordered list of columns scanned from the blueprints table.
const allColumns = `id, name, provider, manifests, created_at, updated_at`

// scanBlueprint scans a single Blueprint from a row.
func scanBlueprint(row pgx.Row) (*Blueprint, error) {
	var bp Blueprint
	err := row.Scan(
		&bp.ID, &bp.Name, &bp.Provider, &bp.Manifests,
		&bp.CreatedAt, &bp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, fmt.Errorf("scanning blueprint row: %w", err)
	}
	return &bp, nil
}

// Create inserts a new blueprint record.
func (r *PostgresRepository) Create(ctx context.Context, bp *Blueprint) error {
	query := fmt.Sprintf(`
		INSERT INTO blueprints (name, provider, manifests)
		VALUES ($1, $2, $3)
		RETURNING %s`, allColumns)

	row := r.pool.QueryRow(ctx, query, bp.Name, bp.Provider, bp.Manifests)

	created, err := scanBlueprint(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateBlueprintName
		}
		return fmt.Errorf("inserting blueprint: %w", err)
	}

	*bp = *created
	return nil
}

// GetByID retrieves a single blueprint by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Blueprint, error) {
	query := fmt.Sprintf(`SELECT %s FROM blueprints WHERE id = $1`, allColumns)
	return scanBlueprint(r.pool.QueryRow(ctx, query, id))
}

// GetByName retrieves a single blueprint by its name.
func (r *PostgresRepository) GetByName(ctx context.Context, name string) (*Blueprint, error) {
	query := fmt.Sprintf(`SELECT %s FROM blueprints WHERE name = $1`, allColumns)
	return scanBlueprint(r.pool.QueryRow(ctx, query, name))
}

// List retrieves all blueprints ordered by creation time.
func (r *PostgresRepository) List(ctx context.Context) ([]Blueprint, error) {
	query := fmt.Sprintf(`SELECT %s FROM blueprints ORDER BY created_at ASC`, allColumns)

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing blueprints: %w", err)
	}
	defer rows.Close()

	var blueprints []Blueprint
	for rows.Next() {
		var bp Blueprint
		err := rows.Scan(
			&bp.ID, &bp.Name, &bp.Provider, &bp.Manifests,
			&bp.CreatedAt, &bp.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning blueprint row: %w", err)
		}
		blueprints = append(blueprints, bp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating blueprint rows: %w", err)
	}

	if blueprints == nil {
		blueprints = []Blueprint{}
	}

	return blueprints, nil
}

// Delete removes a blueprint by its UUID. Returns ErrBlueprintHasTiers if the
// blueprint is referenced by any tier (FK violation).
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM blueprints WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrBlueprintHasTiers
		}
		return fmt.Errorf("deleting blueprint: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrBlueprintNotFound
	}

	return nil
}
