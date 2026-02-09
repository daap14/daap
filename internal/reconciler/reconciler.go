package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
)

// watchedStatuses are the database statuses the reconciler monitors.
var watchedStatuses = []string{"provisioning", "ready", "error"}

// Reconciler polls databases and reconciles their state with Kubernetes cluster status.
type Reconciler struct {
	repo     database.Repository
	k8sMgr   k8s.ResourceManager
	interval time.Duration
}

// New creates a new Reconciler.
func New(repo database.Repository, k8sMgr k8s.ResourceManager, interval time.Duration) *Reconciler {
	return &Reconciler{
		repo:     repo,
		k8sMgr:   k8sMgr,
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
	clusterStatus, err := r.k8sMgr.GetClusterStatus(ctx, db.Namespace, db.ClusterName)
	if err != nil {
		slog.Warn("reconciler: failed to get cluster status",
			"database", db.Name,
			"cluster", db.ClusterName,
			"error", err,
		)
		return
	}

	if clusterStatus.Ready && db.Status != "ready" {
		r.markReady(ctx, db)
		return
	}

	if !clusterStatus.Ready && isFailedPhase(clusterStatus.Phase) && db.Status != "error" {
		r.markError(ctx, db, clusterStatus.Phase)
	}
}

func (r *Reconciler) markReady(ctx context.Context, db *database.Database) {
	secretName := db.ClusterName + "-app"
	host := db.PoolerName + "." + db.Namespace + ".svc.cluster.local"
	port := 5432

	_, err := r.repo.UpdateStatus(ctx, db.ID, database.StatusUpdate{
		Status:     "ready",
		Host:       &host,
		Port:       &port,
		SecretName: &secretName,
	})
	if err != nil {
		slog.Error("reconciler: failed to update database to ready",
			"database", db.Name,
			"error", err,
		)
		return
	}

	slog.Info("reconciler: database is ready", "database", db.Name)
}

func (r *Reconciler) markError(ctx context.Context, db *database.Database, phase string) {
	_, err := r.repo.UpdateStatus(ctx, db.ID, database.StatusUpdate{
		Status: "error",
	})
	if err != nil {
		slog.Error("reconciler: failed to update database to error",
			"database", db.Name,
			"error", err,
		)
		return
	}

	slog.Warn("reconciler: database marked as error", "database", db.Name, "phase", phase)
}

func isFailedPhase(phase string) bool {
	switch phase {
	case "Setting up primary", "Creating primary", "Cluster in healthy state":
		// Transient or healthy phases — not failed.
		return false
	case "Failed", "Error",
		"Cluster in unhealthy state",
		"Failed to create primary",
		"Failed to reconcile":
		return true
	}
	return false
}
