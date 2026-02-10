package auth

import (
	"time"

	"github.com/google/uuid"
)

// User represents a row in the users table.
type User struct {
	ID           uuid.UUID
	Name         string
	TeamID       *uuid.UUID // nil for superuser
	IsSuperuser  bool
	ApiKeyPrefix string
	ApiKeyHash   string
	CreatedAt    time.Time
	RevokedAt    *time.Time
}

// Identity is stored in the request context after authentication.
type Identity struct {
	UserID      uuid.UUID
	UserName    string
	TeamID      *uuid.UUID // nil for superuser
	TeamName    *string    // nil for superuser
	Role        *string    // nil for superuser; "platform" or "product"
	IsSuperuser bool
}
