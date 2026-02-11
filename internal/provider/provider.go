package provider

import (
	"context"

	"github.com/google/uuid"
)

// Provider abstracts infrastructure backends (CNPG, RDS, etc.).
type Provider interface {
	// Apply templates the blueprint manifests and creates/updates all resources.
	Apply(ctx context.Context, db ProviderDatabase, manifests string) error

	// Delete removes all resources associated with a database.
	Delete(ctx context.Context, db ProviderDatabase) error

	// CheckHealth returns the current health status of a database's resources.
	CheckHealth(ctx context.Context, db ProviderDatabase) (HealthResult, error)
}

// ProviderDatabase holds the database fields needed by providers.
type ProviderDatabase struct {
	ID          uuid.UUID
	Name        string
	Namespace   string
	ClusterName string
	PoolerName  string
	OwnerTeam   string
	OwnerTeamID uuid.UUID
	Tier        string
	TierID      uuid.UUID
	Blueprint   string
	Provider    string
}

// HealthResult represents the health status returned by a provider.
type HealthResult struct {
	Status     string  // "provisioning", "ready", "error"
	Host       *string
	Port       *int
	SecretName *string
}
