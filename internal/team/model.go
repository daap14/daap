package team

import (
	"time"

	"github.com/google/uuid"
)

// Team represents a row in the teams table.
type Team struct {
	ID        uuid.UUID
	Name      string
	Role      string // "platform" or "product"
	CreatedAt time.Time
	UpdatedAt time.Time
}
