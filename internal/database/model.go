package database

import (
	"time"

	"github.com/google/uuid"
)

// Database represents a row in the databases table.
type Database struct {
	ID          uuid.UUID
	Name        string
	OwnerTeam   string
	Purpose     string
	Namespace   string
	ClusterName string
	PoolerName  string
	Status      string
	Host        *string
	Port        *int
	SecretName  *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// ListFilter holds optional filters and pagination for listing databases.
type ListFilter struct {
	OwnerTeam *string
	Status    *string
	Name      *string // partial match (ILIKE)
	Page      int     // default 1
	Limit     int     // default 20
}

// ListResult holds the result of a paginated list query.
type ListResult struct {
	Databases []Database
	Total     int
	Page      int
	Limit     int
}

// UpdateFields holds user-updatable fields on a database record.
// Nil fields are not updated.
type UpdateFields struct {
	OwnerTeam *string
	Purpose   *string
}

// StatusUpdate holds fields updated during reconciliation.
type StatusUpdate struct {
	Status     string
	Host       *string
	Port       *int
	SecretName *string
}
