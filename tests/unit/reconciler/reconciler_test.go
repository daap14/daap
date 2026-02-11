package reconciler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/provider"
	"github.com/daap14/daap/internal/reconciler"
	"github.com/daap14/daap/internal/tier"
)

// --- Mock Database Repository ---

type mockRepo struct {
	mu             sync.Mutex
	createFn       func(ctx context.Context, db *database.Database) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*database.Database, error)
	listFn         func(ctx context.Context, filter database.ListFilter) (*database.ListResult, error)
	updateFn       func(ctx context.Context, id uuid.UUID, fields database.UpdateFields) (*database.Database, error)
	updateStatusFn func(ctx context.Context, id uuid.UUID, su database.StatusUpdate) (*database.Database, error)
	softDeleteFn   func(ctx context.Context, id uuid.UUID) error

	statusUpdates []database.StatusUpdate
}

func (m *mockRepo) Create(ctx context.Context, db *database.Database) error {
	if m.createFn != nil {
		return m.createFn(ctx, db)
	}
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*database.Database, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, database.ErrNotFound
}

func (m *mockRepo) List(ctx context.Context, filter database.ListFilter) (*database.ListResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
}

func (m *mockRepo) Update(ctx context.Context, id uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, fields)
	}
	return nil, database.ErrNotFound
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, su database.StatusUpdate) (*database.Database, error) {
	m.mu.Lock()
	m.statusUpdates = append(m.statusUpdates, su)
	m.mu.Unlock()
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, su)
	}
	return &database.Database{ID: id, Status: su.Status}, nil
}

func (m *mockRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if m.softDeleteFn != nil {
		return m.softDeleteFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) getStatusUpdates() []database.StatusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]database.StatusUpdate, len(m.statusUpdates))
	copy(result, m.statusUpdates)
	return result
}

// --- Mock Tier Repository ---

type mockTierRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*tier.Tier, error)
}

func (m *mockTierRepo) Create(_ context.Context, _ *tier.Tier) error { return nil }
func (m *mockTierRepo) GetByID(ctx context.Context, id uuid.UUID) (*tier.Tier, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, tier.ErrTierNotFound
}
func (m *mockTierRepo) GetByName(_ context.Context, _ string) (*tier.Tier, error) {
	return nil, tier.ErrTierNotFound
}
func (m *mockTierRepo) List(_ context.Context) ([]tier.Tier, error) { return nil, nil }
func (m *mockTierRepo) Update(_ context.Context, _ uuid.UUID, _ tier.UpdateFields) (*tier.Tier, error) {
	return nil, tier.ErrTierNotFound
}
func (m *mockTierRepo) Delete(_ context.Context, _ uuid.UUID) error { return nil }

// --- Mock Blueprint Repository ---

type mockBPRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*blueprint.Blueprint, error)
}

func (m *mockBPRepo) Create(_ context.Context, _ *blueprint.Blueprint) error { return nil }
func (m *mockBPRepo) GetByID(ctx context.Context, id uuid.UUID) (*blueprint.Blueprint, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, blueprint.ErrBlueprintNotFound
}
func (m *mockBPRepo) GetByName(_ context.Context, _ string) (*blueprint.Blueprint, error) {
	return nil, blueprint.ErrBlueprintNotFound
}
func (m *mockBPRepo) List(_ context.Context) ([]blueprint.Blueprint, error) { return nil, nil }
func (m *mockBPRepo) Delete(_ context.Context, _ uuid.UUID) error           { return nil }

// --- Mock Provider ---

type mockProvider struct {
	checkHealthFn func(ctx context.Context, db provider.ProviderDatabase) (provider.HealthResult, error)
}

func (m *mockProvider) Apply(_ context.Context, _ provider.ProviderDatabase, _ string) error {
	return nil
}
func (m *mockProvider) Delete(_ context.Context, _ provider.ProviderDatabase) error { return nil }
func (m *mockProvider) CheckHealth(ctx context.Context, db provider.ProviderDatabase) (provider.HealthResult, error) {
	if m.checkHealthFn != nil {
		return m.checkHealthFn(ctx, db)
	}
	return provider.HealthResult{Status: "provisioning"}, nil
}

// --- Helpers ---

var (
	testTierID      = uuid.New()
	testBlueprintID = uuid.New()
)

func provisioningDB(id uuid.UUID, name string) database.Database {
	tierID := testTierID
	return database.Database{
		ID:            id,
		Name:          name,
		OwnerTeamID:   uuid.New(),
		OwnerTeamName: "platform",
		Purpose:       "testing",
		Namespace:     "default",
		ClusterName:   "daap-" + name,
		PoolerName:    "daap-" + name + "-pooler",
		TierID:        &tierID,
		Status:        "provisioning",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func defaultTierRepo() *mockTierRepo {
	bpID := testBlueprintID
	return &mockTierRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*tier.Tier, error) {
			return &tier.Tier{
				ID:          testTierID,
				Name:        "standard",
				BlueprintID: &bpID,
			}, nil
		},
	}
}

func defaultBPRepo() *mockBPRepo {
	return &mockBPRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*blueprint.Blueprint, error) {
			return &blueprint.Blueprint{
				ID:       testBlueprintID,
				Name:     "cnpg-standard",
				Provider: "cnpg",
			}, nil
		},
	}
}

func registryWith(p provider.Provider) *provider.Registry {
	reg := provider.NewRegistry()
	reg.Register("cnpg", p)
	return reg
}

func TestReconcile_ProvisioningToReady(t *testing.T) {
	// Arrange
	id := uuid.New()
	host := "daap-testdb-pooler.default.svc.cluster.local"
	port := 5432
	secret := "daap-testdb-app"

	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "provisioning" {
				db := provisioningDB(id, "testdb")
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1, Page: 1, Limit: 100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			return provider.HealthResult{
				Status:     "ready",
				Host:       &host,
				Port:       &port,
				SecretName: &secret,
			}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act: start the reconciler and let it run one tick
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert
	updates := repo.getStatusUpdates()
	require.GreaterOrEqual(t, len(updates), 1)

	lastUpdate := updates[len(updates)-1]
	assert.Equal(t, "ready", lastUpdate.Status)
	require.NotNil(t, lastUpdate.Host)
	assert.Equal(t, host, *lastUpdate.Host)
	require.NotNil(t, lastUpdate.Port)
	assert.Equal(t, 5432, *lastUpdate.Port)
	require.NotNil(t, lastUpdate.SecretName)
	assert.Equal(t, "daap-testdb-app", *lastUpdate.SecretName)
}

func TestReconcile_ProvisioningToError(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "provisioning" {
				db := provisioningDB(id, "faildb")
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1, Page: 1, Limit: 100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			return provider.HealthResult{Status: "error"}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert
	updates := repo.getStatusUpdates()
	require.GreaterOrEqual(t, len(updates), 1)

	lastUpdate := updates[len(updates)-1]
	assert.Equal(t, "error", lastUpdate.Status)
}

func TestReconcile_ErrorToReady(t *testing.T) {
	// Arrange: database in "error" status recovers when provider reports healthy
	id := uuid.New()
	host := "daap-recover-db-pooler.default.svc.cluster.local"
	port := 5432
	secret := "daap-recover-db-app"

	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "error" {
				db := provisioningDB(id, "recover-db")
				db.Status = "error"
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1, Page: 1, Limit: 100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			return provider.HealthResult{
				Status:     "ready",
				Host:       &host,
				Port:       &port,
				SecretName: &secret,
			}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert: should transition from error to ready
	updates := repo.getStatusUpdates()
	require.GreaterOrEqual(t, len(updates), 1)
	lastUpdate := updates[len(updates)-1]
	assert.Equal(t, "ready", lastUpdate.Status)
	require.NotNil(t, lastUpdate.SecretName)
	assert.Equal(t, "daap-recover-db-app", *lastUpdate.SecretName)
}

func TestReconcile_ReadyToError(t *testing.T) {
	// Arrange: database in "ready" status drifts to error when provider reports unhealthy
	id := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "ready" {
				db := provisioningDB(id, "drift-db")
				db.Status = "ready"
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1, Page: 1, Limit: 100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			return provider.HealthResult{Status: "error"}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert: should transition from ready to error
	updates := repo.getStatusUpdates()
	require.GreaterOrEqual(t, len(updates), 1)
	lastUpdate := updates[len(updates)-1]
	assert.Equal(t, "error", lastUpdate.Status)
}

func TestReconcile_NoDatabases(t *testing.T) {
	// Arrange: empty list returned
	checkHealthCalled := false
	repo := &mockRepo{
		listFn: func(_ context.Context, _ database.ListFilter) (*database.ListResult, error) {
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			checkHealthCalled = true
			return provider.HealthResult{Status: "provisioning"}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert: no provider calls should have been made
	assert.False(t, checkHealthCalled, "expected no provider health check when no databases")
	updates := repo.getStatusUpdates()
	assert.Empty(t, updates, "expected no status updates")
}

func TestReconcile_GracefulShutdown(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	p := &mockProvider{}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 1*time.Hour) // long interval so it won't tick

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	// Act
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	// Assert: Start should return promptly
	select {
	case <-done:
		// success - Start returned after context cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("reconciler did not shut down within 2 seconds after context cancellation")
	}
}

func TestReconcile_DatabaseWithoutTier_Skipped(t *testing.T) {
	// Arrange: database has no tier ID — reconciler should skip it
	id := uuid.New()
	checkHealthCalled := false

	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "provisioning" {
				db := provisioningDB(id, "no-tier-db")
				db.TierID = nil // no tier
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1, Page: 1, Limit: 100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}

	p := &mockProvider{
		checkHealthFn: func(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
			checkHealthCalled = true
			return provider.HealthResult{Status: "provisioning"}, nil
		},
	}

	r := reconciler.New(repo, defaultTierRepo(), defaultBPRepo(), registryWith(p), 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert
	assert.False(t, checkHealthCalled, "expected no provider health check for tier-less database")
	updates := repo.getStatusUpdates()
	assert.Empty(t, updates, "expected no status updates for tier-less database")
}
