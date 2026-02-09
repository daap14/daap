package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a database record is not found.
var ErrNotFound = errors.New("database not found")

// ErrDuplicateName is returned when a database with the same name already exists.
var ErrDuplicateName = errors.New("database name already exists")

// Repository provides CRUD operations on the databases table.
type Repository interface {
	Create(ctx context.Context, db *Database) error
	GetByID(ctx context.Context, id uuid.UUID) (*Database, error)
	List(ctx context.Context, filter ListFilter) (*ListResult, error)
	Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Database, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, su StatusUpdate) (*Database, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// PostgresRepository implements Repository using pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new Repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &PostgresRepository{pool: pool}
}

// Create inserts a new database record. It auto-generates cluster_name and pooler_name
// from the database name, and sets status to "provisioning".
func (r *PostgresRepository) Create(ctx context.Context, db *Database) error {
	db.ClusterName = fmt.Sprintf("daap-%s", db.Name)
	db.PoolerName = fmt.Sprintf("daap-%s-pooler", db.Name)
	if db.Status == "" {
		db.Status = "provisioning"
	}

	query := `
		INSERT INTO databases (name, owner_team, purpose, namespace, cluster_name, pooler_name, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		db.Name,
		db.OwnerTeam,
		db.Purpose,
		db.Namespace,
		db.ClusterName,
		db.PoolerName,
		db.Status,
	).Scan(&db.ID, &db.CreatedAt, &db.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateName
		}
		return fmt.Errorf("inserting database: %w", err)
	}

	return nil
}

// GetByID retrieves a single non-deleted database by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Database, error) {
	query := `
		SELECT id, name, owner_team, purpose, namespace, cluster_name, pooler_name,
		       status, host, port, secret_name, created_at, updated_at, deleted_at
		FROM databases
		WHERE id = $1 AND deleted_at IS NULL`

	return r.scanOne(ctx, query, id)
}

// List retrieves a paginated, filtered list of non-deleted databases.
func (r *PostgresRepository) List(ctx context.Context, filter ListFilter) (*ListResult, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if filter.OwnerTeam != nil {
		conditions = append(conditions, fmt.Sprintf("owner_team = $%d", argIdx))
		args = append(args, *filter.OwnerTeam)
		argIdx++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Name != nil {
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", argIdx))
		args = append(args, "%"+*filter.Name+"%")
		argIdx++
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM databases %s", whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("counting databases: %w", err)
	}

	offset := (filter.Page - 1) * filter.Limit

	dataQuery := fmt.Sprintf(`
		SELECT id, name, owner_team, purpose, namespace, cluster_name, pooler_name,
		       status, host, port, secret_name, created_at, updated_at, deleted_at
		FROM databases
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, filter.Limit, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}
	defer rows.Close()

	var databases []Database
	for rows.Next() {
		var db Database
		err := rows.Scan(
			&db.ID, &db.Name, &db.OwnerTeam, &db.Purpose, &db.Namespace,
			&db.ClusterName, &db.PoolerName, &db.Status,
			&db.Host, &db.Port, &db.SecretName,
			&db.CreatedAt, &db.UpdatedAt, &db.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning database row: %w", err)
		}
		databases = append(databases, db)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating database rows: %w", err)
	}

	if databases == nil {
		databases = []Database{}
	}

	return &ListResult{
		Databases: databases,
		Total:     total,
		Page:      filter.Page,
		Limit:     filter.Limit,
	}, nil
}

// Update modifies user-updatable fields (owner_team, purpose) on a non-deleted database.
func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Database, error) {
	var setClauses []string
	var args []any
	argIdx := 1

	if fields.OwnerTeam != nil {
		setClauses = append(setClauses, fmt.Sprintf("owner_team = $%d", argIdx))
		args = append(args, *fields.OwnerTeam)
		argIdx++
	}
	if fields.Purpose != nil {
		setClauses = append(setClauses, fmt.Sprintf("purpose = $%d", argIdx))
		args = append(args, *fields.Purpose)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE databases
		SET %s
		WHERE id = $%d AND deleted_at IS NULL
		RETURNING id, name, owner_team, purpose, namespace, cluster_name, pooler_name,
		          status, host, port, secret_name, created_at, updated_at, deleted_at`,
		strings.Join(setClauses, ", "), argIdx)

	return r.scanOne(ctx, query, args...)
}

// UpdateStatus updates the status and connection details of a database record (used by the reconciler).
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, su StatusUpdate) (*Database, error) {
	var setClauses []string
	var args []any
	argIdx := 1

	setClauses = append(setClauses, fmt.Sprintf("status = $%d", argIdx))
	args = append(args, su.Status)
	argIdx++

	if su.Host != nil {
		setClauses = append(setClauses, fmt.Sprintf("host = $%d", argIdx))
		args = append(args, *su.Host)
		argIdx++
	}
	if su.Port != nil {
		setClauses = append(setClauses, fmt.Sprintf("port = $%d", argIdx))
		args = append(args, *su.Port)
		argIdx++
	}
	if su.SecretName != nil {
		setClauses = append(setClauses, fmt.Sprintf("secret_name = $%d", argIdx))
		args = append(args, *su.SecretName)
		argIdx++
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE databases
		SET %s
		WHERE id = $%d AND deleted_at IS NULL
		RETURNING id, name, owner_team, purpose, namespace, cluster_name, pooler_name,
		          status, host, port, secret_name, created_at, updated_at, deleted_at`,
		strings.Join(setClauses, ", "), argIdx)

	return r.scanOne(ctx, query, args...)
}

// SoftDelete marks a database as deleted by setting deleted_at and status to 'deleted'.
func (r *PostgresRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE databases
		SET deleted_at = $1, status = 'deleted', updated_at = $1
		WHERE id = $2 AND deleted_at IS NULL`

	now := time.Now().UTC()
	result, err := r.pool.Exec(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("soft deleting database: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// scanOne scans a single Database row from a query. Returns ErrNotFound if no rows.
func (r *PostgresRepository) scanOne(ctx context.Context, query string, args ...any) (*Database, error) {
	var db Database
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&db.ID, &db.Name, &db.OwnerTeam, &db.Purpose, &db.Namespace,
		&db.ClusterName, &db.PoolerName, &db.Status,
		&db.Host, &db.Port, &db.SecretName,
		&db.CreatedAt, &db.UpdatedAt, &db.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scanning database row: %w", err)
	}
	return &db, nil
}
