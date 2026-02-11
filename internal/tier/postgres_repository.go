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

// allColumns is the ordered list of columns scanned from the tiers table.
const allColumns = `id, name, description, instances, cpu, memory, storage_size,
	storage_class, pg_version, pool_mode, max_connections,
	destruction_strategy, backup_enabled, created_at, updated_at`

// scanTier scans a single Tier from a row.
func scanTier(row pgx.Row) (*Tier, error) {
	var t Tier
	err := row.Scan(
		&t.ID, &t.Name, &t.Description,
		&t.Instances, &t.CPU, &t.Memory, &t.StorageSize,
		&t.StorageClass, &t.PGVersion, &t.PoolMode, &t.MaxConnections,
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
		INSERT INTO tiers (name, description, instances, cpu, memory, storage_size,
			storage_class, pg_version, pool_mode, max_connections,
			destruction_strategy, backup_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING %s`, allColumns)

	row := r.pool.QueryRow(ctx, query,
		t.Name, t.Description,
		t.Instances, t.CPU, t.Memory, t.StorageSize,
		t.StorageClass, t.PGVersion, t.PoolMode, t.MaxConnections,
		t.DestructionStrategy, t.BackupEnabled,
	)

	created, err := scanTier(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateTierName
		}
		return fmt.Errorf("inserting tier: %w", err)
	}

	*t = *created
	return nil
}

// GetByID retrieves a single tier by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Tier, error) {
	query := fmt.Sprintf(`SELECT %s FROM tiers WHERE id = $1`, allColumns)
	return scanTier(r.pool.QueryRow(ctx, query, id))
}

// GetByName retrieves a single tier by its name.
func (r *PostgresRepository) GetByName(ctx context.Context, name string) (*Tier, error) {
	query := fmt.Sprintf(`SELECT %s FROM tiers WHERE name = $1`, allColumns)
	return scanTier(r.pool.QueryRow(ctx, query, name))
}

// List retrieves all tiers ordered by creation time.
func (r *PostgresRepository) List(ctx context.Context) ([]Tier, error) {
	query := fmt.Sprintf(`SELECT %s FROM tiers ORDER BY created_at ASC`, allColumns)

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
			&t.Instances, &t.CPU, &t.Memory, &t.StorageSize,
			&t.StorageClass, &t.PGVersion, &t.PoolMode, &t.MaxConnections,
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
	if fields.Instances != nil {
		setClauses = append(setClauses, fmt.Sprintf("instances = $%d", argIdx))
		args = append(args, *fields.Instances)
		argIdx++
	}
	if fields.CPU != nil {
		setClauses = append(setClauses, fmt.Sprintf("cpu = $%d", argIdx))
		args = append(args, *fields.CPU)
		argIdx++
	}
	if fields.Memory != nil {
		setClauses = append(setClauses, fmt.Sprintf("memory = $%d", argIdx))
		args = append(args, *fields.Memory)
		argIdx++
	}
	if fields.StorageSize != nil {
		setClauses = append(setClauses, fmt.Sprintf("storage_size = $%d", argIdx))
		args = append(args, *fields.StorageSize)
		argIdx++
	}
	if fields.StorageClass != nil {
		setClauses = append(setClauses, fmt.Sprintf("storage_class = $%d", argIdx))
		args = append(args, *fields.StorageClass)
		argIdx++
	}
	if fields.PGVersion != nil {
		setClauses = append(setClauses, fmt.Sprintf("pg_version = $%d", argIdx))
		args = append(args, *fields.PGVersion)
		argIdx++
	}
	if fields.PoolMode != nil {
		setClauses = append(setClauses, fmt.Sprintf("pool_mode = $%d", argIdx))
		args = append(args, *fields.PoolMode)
		argIdx++
	}
	if fields.MaxConnections != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_connections = $%d", argIdx))
		args = append(args, *fields.MaxConnections)
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
		RETURNING %s`,
		strings.Join(setClauses, ", "), argIdx, allColumns)

	t, err := scanTier(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		return nil, err
	}
	return t, nil
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
