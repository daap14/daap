package blueprint

import (
	"time"

	"github.com/google/uuid"
)

// Blueprint represents a row in the blueprints table.
type Blueprint struct {
	ID        uuid.UUID
	Name      string
	Provider  string
	Manifests string
	CreatedAt time.Time
	UpdatedAt time.Time
}
