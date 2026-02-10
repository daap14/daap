package team

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

// NewRepository creates a new Repository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &PostgresRepository{pool: pool}
}

// Create inserts a new team record.
func (r *PostgresRepository) Create(ctx context.Context, t *Team) error {
	query := `
		INSERT INTO teams (name, role)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query, t.Name, t.Role).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateTeamName
		}
		return fmt.Errorf("inserting team: %w", err)
	}

	return nil
}

// GetByID retrieves a single team by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Team, error) {
	query := `
		SELECT id, name, role, created_at, updated_at
		FROM teams
		WHERE id = $1`

	var t Team
	err := r.pool.QueryRow(ctx, query, id).Scan(&t.ID, &t.Name, &t.Role, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("querying team: %w", err)
	}

	return &t, nil
}

// List retrieves all teams ordered by creation time.
func (r *PostgresRepository) List(ctx context.Context) ([]Team, error) {
	query := `
		SELECT id, name, role, created_at, updated_at
		FROM teams
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var t Team
		err := rows.Scan(&t.ID, &t.Name, &t.Role, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning team row: %w", err)
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating team rows: %w", err)
	}

	if teams == nil {
		teams = []Team{}
	}

	return teams, nil
}

// Delete removes a team by its UUID. Returns ErrTeamHasUsers if the team
// still has users referencing it (FK RESTRICT).
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM teams WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrTeamHasUsers
		}
		return fmt.Errorf("deleting team: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrTeamNotFound
	}

	return nil
}
