package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrUserNotFound is returned when a user record is not found.
var ErrUserNotFound = errors.New("user not found")

// ErrUserRevoked is returned when attempting to operate on a revoked user.
var ErrUserRevoked = errors.New("user is revoked")

// UserRepository provides operations on the users table.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByPrefix(ctx context.Context, prefix string) ([]User, error)
	List(ctx context.Context) ([]User, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	CountAll(ctx context.Context) (int, error)
}
