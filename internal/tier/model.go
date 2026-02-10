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
	Instances           int
	CPU                 string
	Memory              string
	StorageSize         string
	StorageClass        string
	PGVersion           string
	PoolMode            string
	MaxConnections      int
	DestructionStrategy string
	BackupEnabled       bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UpdateFields holds optional fields for a partial tier update.
// Nil fields are not updated.
type UpdateFields struct {
	Description         *string
	Instances           *int
	CPU                 *string
	Memory              *string
	StorageSize         *string
	StorageClass        *string
	PGVersion           *string
	PoolMode            *string
	MaxConnections      *int
	DestructionStrategy *string
	BackupEnabled       *bool
}
