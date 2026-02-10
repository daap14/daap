package team

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrTeamNotFound is returned when a team record is not found.
var ErrTeamNotFound = errors.New("team not found")

// ErrDuplicateTeamName is returned when a team with the same name already exists.
var ErrDuplicateTeamName = errors.New("team name already exists")

// ErrTeamHasUsers is returned when attempting to delete a team that still has users.
var ErrTeamHasUsers = errors.New("team has users")

// Repository provides CRUD operations on the teams table.
type Repository interface {
	Create(ctx context.Context, team *Team) error
	GetByID(ctx context.Context, id uuid.UUID) (*Team, error)
	GetByName(ctx context.Context, name string) (*Team, error)
	List(ctx context.Context) ([]Team, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
