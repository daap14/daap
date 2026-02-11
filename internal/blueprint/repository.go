package blueprint

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrBlueprintNotFound is returned when a blueprint record is not found.
var ErrBlueprintNotFound = errors.New("blueprint not found")

// ErrDuplicateBlueprintName is returned when a blueprint with the same name already exists.
var ErrDuplicateBlueprintName = errors.New("blueprint name already exists")

// ErrBlueprintHasTiers is returned when attempting to delete a blueprint that is referenced by tiers.
var ErrBlueprintHasTiers = errors.New("blueprint has tiers")

// Repository provides CRUD operations on the blueprints table.
// Blueprints are immutable — there is no Update method.
type Repository interface {
	Create(ctx context.Context, bp *Blueprint) error
	GetByID(ctx context.Context, id uuid.UUID) (*Blueprint, error)
	GetByName(ctx context.Context, name string) (*Blueprint, error)
	List(ctx context.Context) ([]Blueprint, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
