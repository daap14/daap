package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/provider"
	"github.com/daap14/daap/internal/tier"
)

// watchedStatuses are the database statuses the reconciler monitors.
var watchedStatuses = []string{"provisioning", "ready", "error"}

// Reconciler polls databases and reconciles their state with provider health checks.
type Reconciler struct {
	repo     database.Repository
	tierRepo tier.Repository
	bpRepo   blueprint.Repository
	registry *provider.Registry
	interval time.Duration
}

// New creates a new Reconciler.
func New(repo database.Repository, tierRepo tier.Repository, bpRepo blueprint.Repository, registry *provider.Registry, interval time.Duration) *Reconciler {
	return &Reconciler{
		repo:     repo,
		tierRepo: tierRepo,
		bpRepo:   bpRepo,
		registry: registry,
		interval: interval,
	}
}

// Start begins the reconciliation loop. It blocks until ctx is cancelled.
func (r *Reconciler) Start(ctx context.Context) {
	slog.Info("reconciler started", "interval", r.interval.String())
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciler stopped")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	for _, status := range watchedStatuses {
		if ctx.Err() != nil {
			return
		}
		r.reconcileByStatus(ctx, status)
	}
}

func (r *Reconciler) reconcileByStatus(ctx context.Context, status string) {
	s := status
	result, err := r.repo.List(ctx, database.ListFilter{
		Status: &s,
		Page:   1,
		Limit:  100,
	})
	if err != nil {
		slog.Error("reconciler: failed to list databases", "status", status, "error", err)
		return
	}

	for _, db := range result.Databases {
		if ctx.Err() != nil {
			return
		}
		r.reconcileOne(ctx, &db)
	}
}

func (r *Reconciler) reconcileOne(ctx context.Context, db *database.Database) {
	if db.TierID == nil {
		slog.Warn("reconciler: database has no tier, skipping", "database", db.Name)
		return
	}

	t, err := r.tierRepo.GetByID(ctx, *db.TierID)
	if err != nil {
		slog.Warn("reconciler: failed to get tier", "database", db.Name, "tierID", db.TierID, "error", err)
		return
	}

	if t.BlueprintID == nil {
		slog.Warn("reconciler: tier has no blueprint, skipping", "database", db.Name, "tier", t.Name)
		return
	}

	bp, err := r.bpRepo.GetByID(ctx, *t.BlueprintID)
	if err != nil {
		slog.Warn("reconciler: failed to get blueprint", "database", db.Name, "blueprintID", t.BlueprintID, "error", err)
		return
	}

	p, ok := r.registry.Get(bp.Provider)
	if !ok {
		slog.Warn("reconciler: provider not registered", "database", db.Name, "provider", bp.Provider)
		return
	}

	pdb := toProviderDatabase(db, t, bp)

	healthResult, err := p.CheckHealth(ctx, pdb)
	if err != nil {
		slog.Warn("reconciler: health check failed",
			"database", db.Name,
			"provider", bp.Provider,
			"error", err,
		)
		return
	}

	switch healthResult.Status {
	case "ready":
		if db.Status != "ready" {
			su := database.StatusUpdate{
				Status:     "ready",
				Host:       healthResult.Host,
				Port:       healthResult.Port,
				SecretName: healthResult.SecretName,
			}
			if _, err := r.repo.UpdateStatus(ctx, db.ID, su); err != nil {
				slog.Error("reconciler: failed to update database to ready",
					"database", db.Name, "error", err)
				return
			}
			slog.Info("reconciler: database is ready", "database", db.Name)
		}
	case "error":
		if db.Status != "error" {
			su := database.StatusUpdate{Status: "error"}
			if _, err := r.repo.UpdateStatus(ctx, db.ID, su); err != nil {
				slog.Error("reconciler: failed to update database to error",
					"database", db.Name, "error", err)
				return
			}
			slog.Warn("reconciler: database marked as error", "database", db.Name)
		}
	default:
		// "provisioning" or unknown — no status change needed
	}
}

// toProviderDatabase builds a ProviderDatabase from domain models.
func toProviderDatabase(db *database.Database, t *tier.Tier, bp *blueprint.Blueprint) provider.ProviderDatabase {
	return provider.ProviderDatabase{
		ID:          db.ID,
		Name:        db.Name,
		Namespace:   db.Namespace,
		ClusterName: db.ClusterName,
		PoolerName:  db.PoolerName,
		OwnerTeam:   db.OwnerTeamName,
		OwnerTeamID: db.OwnerTeamID,
		Tier:        t.Name,
		TierID:      t.ID,
		Blueprint:   bp.Name,
		Provider:    bp.Provider,
	}
}
