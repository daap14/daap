package reconciler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/reconciler"
)

// --- Mock Repository ---

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

// --- Mock ResourceManager ---

type mockManager struct {
	applyClusterFn     func(ctx context.Context, cluster *unstructured.Unstructured) error
	applyPoolerFn      func(ctx context.Context, pooler *unstructured.Unstructured) error
	deleteClusterFn    func(ctx context.Context, namespace, name string) error
	deletePoolerFn     func(ctx context.Context, namespace, name string) error
	getClusterStatusFn func(ctx context.Context, namespace, name string) (k8s.ClusterStatus, error)
	getSecretFn        func(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

func (m *mockManager) ApplyCluster(ctx context.Context, cluster *unstructured.Unstructured) error {
	if m.applyClusterFn != nil {
		return m.applyClusterFn(ctx, cluster)
	}
	return nil
}

func (m *mockManager) ApplyPooler(ctx context.Context, pooler *unstructured.Unstructured) error {
	if m.applyPoolerFn != nil {
		return m.applyPoolerFn(ctx, pooler)
	}
	return nil
}

func (m *mockManager) DeleteCluster(ctx context.Context, namespace, name string) error {
	if m.deleteClusterFn != nil {
		return m.deleteClusterFn(ctx, namespace, name)
	}
	return nil
}

func (m *mockManager) DeletePooler(ctx context.Context, namespace, name string) error {
	if m.deletePoolerFn != nil {
		return m.deletePoolerFn(ctx, namespace, name)
	}
	return nil
}

func (m *mockManager) GetClusterStatus(ctx context.Context, namespace, name string) (k8s.ClusterStatus, error) {
	if m.getClusterStatusFn != nil {
		return m.getClusterStatusFn(ctx, namespace, name)
	}
	return k8s.ClusterStatus{}, nil
}

func (m *mockManager) GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	if m.getSecretFn != nil {
		return m.getSecretFn(ctx, namespace, name)
	}
	return nil, nil
}

// --- Helpers ---

func provisioningDB(id uuid.UUID, name string) database.Database {
	return database.Database{
		ID:          id,
		Name:        name,
		OwnerTeam:   "platform",
		Purpose:     "testing",
		Namespace:   "default",
		ClusterName: "daap-" + name,
		PoolerName:  "daap-" + name + "-pooler",
		Status:      "provisioning",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func TestReconcile_ProvisioningToReady(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "provisioning" {
				db := provisioningDB(id, "testdb")
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1,
					Page:      1,
					Limit:     100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}
	mgr := &mockManager{
		getClusterStatusFn: func(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
			return k8s.ClusterStatus{Phase: "Cluster in healthy state", Ready: true}, nil
		},
	}

	r := reconciler.New(repo, mgr, 50*time.Millisecond)

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
	assert.Contains(t, *lastUpdate.Host, "daap-testdb-pooler.default.svc.cluster.local")
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
					Total:     1,
					Page:      1,
					Limit:     100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}
	mgr := &mockManager{
		getClusterStatusFn: func(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
			return k8s.ClusterStatus{Phase: "Cluster in unhealthy state", Ready: false}, nil
		},
	}

	r := reconciler.New(repo, mgr, 50*time.Millisecond)

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
	// Arrange: database in "error" status recovers when cluster becomes healthy
	id := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "error" {
				db := provisioningDB(id, "recover-db")
				db.Status = "error"
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1,
					Page:      1,
					Limit:     100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}
	mgr := &mockManager{
		getClusterStatusFn: func(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
			return k8s.ClusterStatus{Phase: "Cluster in healthy state", Ready: true}, nil
		},
	}

	r := reconciler.New(repo, mgr, 50*time.Millisecond)

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
	// Arrange: database in "ready" status drifts to error when cluster becomes unhealthy
	id := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			if filter.Status != nil && *filter.Status == "ready" {
				db := provisioningDB(id, "drift-db")
				db.Status = "ready"
				return &database.ListResult{
					Databases: []database.Database{db},
					Total:     1,
					Page:      1,
					Limit:     100,
				}, nil
			}
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}
	mgr := &mockManager{
		getClusterStatusFn: func(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
			return k8s.ClusterStatus{Phase: "Cluster in unhealthy state", Ready: false}, nil
		},
	}

	r := reconciler.New(repo, mgr, 50*time.Millisecond)

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

func TestReconcile_NoProvisioningDatabases(t *testing.T) {
	// Arrange: empty list returned
	getClusterCalled := false
	repo := &mockRepo{
		listFn: func(_ context.Context, _ database.ListFilter) (*database.ListResult, error) {
			return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 100}, nil
		},
	}
	mgr := &mockManager{
		getClusterStatusFn: func(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
			getClusterCalled = true
			return k8s.ClusterStatus{}, nil
		},
	}

	r := reconciler.New(repo, mgr, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Act
	go r.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	// Assert: no K8s calls should have been made
	assert.False(t, getClusterCalled, "expected no K8s cluster status check when no provisioning databases")
	updates := repo.getStatusUpdates()
	assert.Empty(t, updates, "expected no status updates")
}

func TestReconcile_GracefulShutdown(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	mgr := &mockManager{}

	r := reconciler.New(repo, mgr, 1*time.Hour) // long interval so it won't tick

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
