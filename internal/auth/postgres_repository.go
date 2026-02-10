package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements UserRepository using pgxpool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new UserRepository backed by the given connection pool.
func NewRepository(pool *pgxpool.Pool) UserRepository {
	return &PostgresRepository{pool: pool}
}

// Create inserts a new user record.
func (r *PostgresRepository) Create(ctx context.Context, u *User) error {
	query := `
		INSERT INTO users (name, team_id, is_superuser, api_key_prefix, api_key_hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err := r.pool.QueryRow(ctx, query,
		u.Name,
		u.TeamID,
		u.IsSuperuser,
		u.ApiKeyPrefix,
		u.ApiKeyHash,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	return nil
}

// GetByID retrieves a single user by its UUID.
func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT id, name, team_id, is_superuser, api_key_prefix, api_key_hash,
		       created_at, revoked_at
		FROM users
		WHERE id = $1`

	var u User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Name, &u.TeamID, &u.IsSuperuser,
		&u.ApiKeyPrefix, &u.ApiKeyHash,
		&u.CreatedAt, &u.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}

	return &u, nil
}

// FindByPrefix returns active (non-revoked) users matching the given API key prefix.
func (r *PostgresRepository) FindByPrefix(ctx context.Context, prefix string) ([]User, error) {
	query := `
		SELECT id, name, team_id, is_superuser, api_key_prefix, api_key_hash,
		       created_at, revoked_at
		FROM users
		WHERE api_key_prefix = $1 AND revoked_at IS NULL`

	rows, err := r.pool.Query(ctx, query, prefix)
	if err != nil {
		return nil, fmt.Errorf("finding users by prefix: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(
			&u.ID, &u.Name, &u.TeamID, &u.IsSuperuser,
			&u.ApiKeyPrefix, &u.ApiKeyHash,
			&u.CreatedAt, &u.RevokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user rows: %w", err)
	}

	if users == nil {
		users = []User{}
	}

	return users, nil
}

// List retrieves all users with team info, ordered by creation time.
// Joins with teams to include team name and role in the result.
func (r *PostgresRepository) List(ctx context.Context) ([]User, error) {
	query := `
		SELECT u.id, u.name, u.team_id, u.is_superuser, u.api_key_prefix,
		       u.api_key_hash, u.created_at, u.revoked_at,
		       t.name, t.role
		FROM users u
		LEFT JOIN teams t ON u.team_id = t.id
		ORDER BY u.created_at ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(
			&u.ID, &u.Name, &u.TeamID, &u.IsSuperuser,
			&u.ApiKeyPrefix, &u.ApiKeyHash,
			&u.CreatedAt, &u.RevokedAt,
			&u.TeamName, &u.TeamRole,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user rows: %w", err)
	}

	if users == nil {
		users = []User{}
	}

	return users, nil
}

// Revoke sets revoked_at on a user. Returns ErrUserNotFound if the user
// does not exist, and ErrUserRevoked if already revoked.
func (r *PostgresRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL`

	result, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("revoking user: %w", err)
	}

	if result.RowsAffected() == 0 {
		// Check if the user exists at all to distinguish not-found from already-revoked
		var exists bool
		err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking user existence: %w", err)
		}
		if !exists {
			return ErrUserNotFound
		}
		return ErrUserRevoked
	}

	return nil
}

// CountAll returns the total number of users in the table (including revoked).
func (r *PostgresRepository) CountAll(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}
