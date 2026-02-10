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

// ErrInvalidOwnerTeam is returned when owner_team_id references a non-existent team.
var ErrInvalidOwnerTeam = errors.New("invalid owner team")

// ErrInvalidTier is returned when tier_id references a non-existent tier.
var ErrInvalidTier = errors.New("invalid tier")

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
		INSERT INTO databases (name, owner_team_id, tier_id, purpose, namespace, cluster_name, pooler_name, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		db.Name,
		db.OwnerTeamID,
		db.TierID,
		db.Purpose,
		db.Namespace,
		db.ClusterName,
		db.PoolerName,
		db.Status,
	).Scan(&db.ID, &db.CreatedAt, &db.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return ErrDuplicateName
			}
			if pgErr.Code == "23503" {
				if strings.Contains(pgErr.ConstraintName, "tier") {
					return ErrInvalidTier
				}
				return ErrInvalidOwnerTeam
			}
		}
		return fmt.Errorf("inserting database: %w", err)
	}

	return nil
}

// GetByID retrieves a single non-deleted database by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Database, error) {
	query := `
		SELECT d.id, d.name, d.owner_team_id, t.name, d.tier_id, COALESCE(tr.name, ''),
		       d.purpose, d.namespace,
		       d.cluster_name, d.pooler_name, d.status,
		       d.host, d.port, d.secret_name,
		       d.created_at, d.updated_at, d.deleted_at
		FROM databases d
		LEFT JOIN teams t ON d.owner_team_id = t.id
		LEFT JOIN tiers tr ON d.tier_id = tr.id
		WHERE d.id = $1 AND d.deleted_at IS NULL`

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

	conditions = append(conditions, "d.deleted_at IS NULL")

	if filter.OwnerTeamID != nil {
		conditions = append(conditions, fmt.Sprintf("d.owner_team_id = $%d", argIdx))
		args = append(args, *filter.OwnerTeamID)
		argIdx++
	}
	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("d.status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Name != nil {
		conditions = append(conditions, fmt.Sprintf("d.name ILIKE $%d", argIdx))
		args = append(args, "%"+*filter.Name+"%")
		argIdx++
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM databases d %s", whereClause)
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("counting databases: %w", err)
	}

	offset := (filter.Page - 1) * filter.Limit

	dataQuery := fmt.Sprintf(`
		SELECT d.id, d.name, d.owner_team_id, t.name, d.tier_id, COALESCE(tr.name, ''),
		       d.purpose, d.namespace,
		       d.cluster_name, d.pooler_name, d.status,
		       d.host, d.port, d.secret_name,
		       d.created_at, d.updated_at, d.deleted_at
		FROM databases d
		LEFT JOIN teams t ON d.owner_team_id = t.id
		LEFT JOIN tiers tr ON d.tier_id = tr.id
		%s
		ORDER BY d.created_at DESC
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
			&db.ID, &db.Name, &db.OwnerTeamID, &db.OwnerTeamName, &db.TierID, &db.TierName,
			&db.Purpose, &db.Namespace,
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

// Update modifies user-updatable fields (owner_team_id, purpose) on a non-deleted database.
func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Database, error) {
	var setClauses []string
	var args []any
	argIdx := 1

	if fields.OwnerTeamID != nil {
		setClauses = append(setClauses, fmt.Sprintf("owner_team_id = $%d", argIdx))
		args = append(args, *fields.OwnerTeamID)
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
		UPDATE databases d
		SET %s
		WHERE d.id = $%d AND d.deleted_at IS NULL
		RETURNING d.id, d.name, d.owner_team_id,
		          (SELECT t.name FROM teams t WHERE t.id = d.owner_team_id),
		          d.tier_id, COALESCE((SELECT tr.name FROM tiers tr WHERE tr.id = d.tier_id), ''),
		          d.purpose, d.namespace, d.cluster_name, d.pooler_name,
		          d.status, d.host, d.port, d.secret_name,
		          d.created_at, d.updated_at, d.deleted_at`,
		strings.Join(setClauses, ", "), argIdx)

	db, err := r.scanOne(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrInvalidOwnerTeam
		}
		return nil, err
	}
	return db, nil
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
		UPDATE databases d
		SET %s
		WHERE d.id = $%d AND d.deleted_at IS NULL
		RETURNING d.id, d.name, d.owner_team_id,
		          (SELECT t.name FROM teams t WHERE t.id = d.owner_team_id),
		          d.tier_id, COALESCE((SELECT tr.name FROM tiers tr WHERE tr.id = d.tier_id), ''),
		          d.purpose, d.namespace, d.cluster_name, d.pooler_name,
		          d.status, d.host, d.port, d.secret_name,
		          d.created_at, d.updated_at, d.deleted_at`,
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
		&db.ID, &db.Name, &db.OwnerTeamID, &db.OwnerTeamName, &db.TierID, &db.TierName,
		&db.Purpose, &db.Namespace,
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
