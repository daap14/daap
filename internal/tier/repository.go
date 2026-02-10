package tier

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrTierNotFound is returned when a tier record is not found.
var ErrTierNotFound = errors.New("tier not found")

// ErrDuplicateTierName is returned when a tier with the same name already exists.
var ErrDuplicateTierName = errors.New("tier name already exists")

// ErrTierHasDatabases is returned when attempting to delete a tier that still has databases.
var ErrTierHasDatabases = errors.New("tier has databases")

// Repository provides CRUD operations on the tiers table.
type Repository interface {
	Create(ctx context.Context, t *Tier) error
	GetByID(ctx context.Context, id uuid.UUID) (*Tier, error)
	GetByName(ctx context.Context, name string) (*Tier, error)
	List(ctx context.Context) ([]Tier, error)
	Update(ctx context.Context, id uuid.UUID, fields UpdateFields) (*Tier, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
