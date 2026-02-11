package tier

import (
	"time"

	"github.com/google/uuid"
)

// Tier represents a row in the tiers table.
type Tier struct {
	ID                  uuid.UUID
	Name                string
	Description         string
	BlueprintID         *uuid.UUID // nullable for migration period
	BlueprintName       string     // transient, populated via JOIN
	DestructionStrategy string
	BackupEnabled       bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UpdateFields holds optional fields for a partial tier update.
// Nil fields are not updated.
type UpdateFields struct {
	Description         *string
	BlueprintID         *uuid.UUID
	DestructionStrategy *string
	BackupEnabled       *bool
}
