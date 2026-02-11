package tier

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// allColumns is the ordered list of columns scanned from the tiers table
// with a LEFT JOIN on blueprints for the transient BlueprintName field.
const allColumns = `t.id, t.name, t.description, t.blueprint_id,
	COALESCE(b.name, ''), t.destruction_strategy, t.backup_enabled,
	t.created_at, t.updated_at`

// fromClause is the common FROM + JOIN clause used by all read queries.
const fromClause = `FROM tiers t LEFT JOIN blueprints b ON t.blueprint_id = b.id`

// scanTier scans a single Tier from a row.
func scanTier(row pgx.Row) (*Tier, error) {
	var t Tier
	err := row.Scan(
		&t.ID, &t.Name, &t.Description,
		&t.BlueprintID, &t.BlueprintName,
		&t.DestructionStrategy, &t.BackupEnabled,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTierNotFound
		}
		return nil, fmt.Errorf("scanning tier row: %w", err)
	}
	return &t, nil
}

// Create inserts a new tier record.
func (r *PostgresRepository) Create(ctx context.Context, t *Tier) error {
	query := fmt.Sprintf(`
		INSERT INTO tiers (name, description, blueprint_id, destruction_strategy, backup_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`)

	var id uuid.UUID
	err := r.pool.QueryRow(ctx, query,
		t.Name, t.Description, t.BlueprintID,
		t.DestructionStrategy, t.BackupEnabled,
	).Scan(&id, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateTierName
		}
		return fmt.Errorf("inserting tier: %w", err)
	}

	t.ID = id

	// Fetch the full record with BlueprintName via JOIN.
	created, err := r.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching created tier: %w", err)
	}
	*t = *created
	return nil
}

// GetByID retrieves a single tier by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Tier, error) {
	query := fmt.Sprintf(`SELECT %s %s WHERE t.id = $1`, allColumns, fromClause)
	return scanTier(r.pool.QueryRow(ctx, query, id))
}

// GetByName retrieves a single tier by its name.
func (r *PostgresRepository) GetByName(ctx context.Context, name string) (*Tier, error) {
	query := fmt.Sprintf(`SELECT %s %s WHERE t.name = $1`, allColumns, fromClause)
	return scanTier(r.pool.QueryRow(ctx, query, name))
}

// List retrieves all tiers ordered by creation time.
func (r *PostgresRepository) List(ctx context.Context) ([]Tier, error) {
	query := fmt.Sprintf(`SELECT %s %s ORDER BY t.created_at ASC`, allColumns, fromClause)

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing tiers: %w", err)
	}
	defer rows.Close()

	var tiers []Tier
	for rows.Next() {
		var t Tier
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description,
			&t.BlueprintID, &t.BlueprintName,
			&t.DestructionStrategy, &t.BackupEnabled,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tier row: %w", err)
		}
		tiers = append(tiers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tier rows: %w", err)
	}

	if tiers == nil {
		tiers = []Tier{}
	}

	return tiers, nil
}

// Update modifies non-nil fields on a tier. Returns the updated tier.
func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Tier, error) {
	var setClauses []string
	var args []any
	argIdx := 1

	if fields.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *fields.Description)
		argIdx++
	}
	if fields.BlueprintID != nil {
		setClauses = append(setClauses, fmt.Sprintf("blueprint_id = $%d", argIdx))
		args = append(args, *fields.BlueprintID)
		argIdx++
	}
	if fields.DestructionStrategy != nil {
		setClauses = append(setClauses, fmt.Sprintf("destruction_strategy = $%d", argIdx))
		args = append(args, *fields.DestructionStrategy)
		argIdx++
	}
	if fields.BackupEnabled != nil {
		setClauses = append(setClauses, fmt.Sprintf("backup_enabled = $%d", argIdx))
		args = append(args, *fields.BackupEnabled)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE tiers
		SET %s
		WHERE id = $%d
		RETURNING id`,
		strings.Join(setClauses, ", "), argIdx)

	var updatedID uuid.UUID
	err := r.pool.QueryRow(ctx, query, args...).Scan(&updatedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTierNotFound
		}
		return nil, fmt.Errorf("updating tier: %w", err)
	}

	// Fetch the full record with BlueprintName via JOIN.
	return r.GetByID(ctx, updatedID)
}

// Delete removes a tier by its UUID. Returns ErrTierHasDatabases if the tier
// still has active (non-soft-deleted) databases referencing it.
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM databases WHERE tier_id = $1 AND deleted_at IS NULL`, id,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("checking active databases for tier: %w", err)
	}
	if count > 0 {
		return ErrTierHasDatabases
	}

	result, err := r.pool.Exec(ctx, `DELETE FROM tiers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting tier: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrTierNotFound
	}

	return nil
}
