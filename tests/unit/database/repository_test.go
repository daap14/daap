package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/database"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupRepo(t *testing.T) (database.Repository, func()) {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultTestDatabaseURL
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: cannot connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping: cannot ping test database: %v", err)
	}

	// Truncate the table for a clean slate
	_, err = pool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)

	repo := database.NewRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return repo, cleanup
}

func newTestDB(name, ownerTeam, namespace string) *database.Database {
	return &database.Database{
		Name:      name,
		OwnerTeam: ownerTeam,
		Purpose:   "test purpose",
		Namespace: namespace,
	}
}

func strPtr(s string) *string { return &s }

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("testdb", "platform", "default")

	err := repo.Create(ctx, db)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, db.ID)
	assert.Equal(t, "daap-testdb", db.ClusterName)
	assert.Equal(t, "daap-testdb-pooler", db.PoolerName)
	assert.Equal(t, "provisioning", db.Status)
	assert.False(t, db.CreatedAt.IsZero())
	assert.False(t, db.UpdatedAt.IsZero())
}

func TestCreate_DuplicateName(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db1 := newTestDB("dupdb", "platform", "default")
	err := repo.Create(ctx, db1)
	require.NoError(t, err)

	db2 := newTestDB("dupdb", "other-team", "default")
	err = repo.Create(ctx, db2)
	assert.ErrorIs(t, err, database.ErrDuplicateName, "duplicate active name should return ErrDuplicateName")
}

func TestCreate_SetsClusterAndPoolerNames(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("my-app", "backend", "staging")

	err := repo.Create(ctx, db)
	require.NoError(t, err)

	assert.Equal(t, "daap-my-app", db.ClusterName)
	assert.Equal(t, "daap-my-app-pooler", db.PoolerName)
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("gettest", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, db.ID)
	require.NoError(t, err)

	assert.Equal(t, db.ID, found.ID)
	assert.Equal(t, "gettest", found.Name)
	assert.Equal(t, "platform", found.OwnerTeam)
	assert.Equal(t, "test purpose", found.Purpose)
	assert.Equal(t, "default", found.Namespace)
	assert.Equal(t, "provisioning", found.Status)
	assert.Nil(t, found.DeletedAt)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())

	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestGetByID_ExcludesSoftDeleted(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("deleted-db", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, db.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
}

// --- List Tests ---

func TestList_EmptyResult(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	result, err := repo.List(ctx, database.ListFilter{})
	require.NoError(t, err)

	assert.Empty(t, result.Databases)
	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 20, result.Limit)
}

func TestList_ReturnsAllRecords(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		db := newTestDB("listdb-"+string(rune('a'+i)), "platform", "default")
		err := repo.Create(ctx, db)
		require.NoError(t, err)
	}

	result, err := repo.List(ctx, database.ListFilter{})
	require.NoError(t, err)

	assert.Equal(t, 3, result.Total)
	assert.Len(t, result.Databases, 3)
}

func TestList_FilterByOwnerTeam(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	for _, team := range []string{"backend", "backend", "frontend"} {
		db := newTestDB("filter-"+team+"-"+uuid.New().String()[:8], team, "default")
		err := repo.Create(ctx, db)
		require.NoError(t, err)
	}

	result, err := repo.List(ctx, database.ListFilter{OwnerTeam: strPtr("backend")})
	require.NoError(t, err)

	assert.Equal(t, 2, result.Total)
	for _, db := range result.Databases {
		assert.Equal(t, "backend", db.OwnerTeam)
	}
}

func TestList_FilterByStatus(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db1 := newTestDB("status-a", "platform", "default")
	err := repo.Create(ctx, db1)
	require.NoError(t, err)

	db2 := newTestDB("status-b", "platform", "default")
	err = repo.Create(ctx, db2)
	require.NoError(t, err)

	// Both are "provisioning" by default
	result, err := repo.List(ctx, database.ListFilter{Status: strPtr("provisioning")})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)

	result, err = repo.List(ctx, database.ListFilter{Status: strPtr("ready")})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}

func TestList_FilterByName(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	for _, name := range []string{"analytics-prod", "analytics-staging", "orders-prod"} {
		db := newTestDB(name, "platform", "default")
		err := repo.Create(ctx, db)
		require.NoError(t, err)
	}

	result, err := repo.List(ctx, database.ListFilter{Name: strPtr("analytics")})
	require.NoError(t, err)

	assert.Equal(t, 2, result.Total)
	for _, db := range result.Databases {
		assert.Contains(t, db.Name, "analytics")
	}
}

func TestList_Pagination(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		db := newTestDB("page-"+string(rune('a'+i)), "platform", "default")
		err := repo.Create(ctx, db)
		require.NoError(t, err)
		// Small sleep to ensure distinct created_at for ordering
		time.Sleep(10 * time.Millisecond)
	}

	// Page 1, limit 2
	result, err := repo.List(ctx, database.ListFilter{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, result.Total)
	assert.Len(t, result.Databases, 2)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 2, result.Limit)

	// Page 2, limit 2
	result2, err := repo.List(ctx, database.ListFilter{Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, result2.Databases, 2)

	// Page 3, limit 2 (only 1 remaining)
	result3, err := repo.List(ctx, database.ListFilter{Page: 3, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, result3.Databases, 1)

	// Ensure no overlap between pages
	allIDs := make(map[uuid.UUID]bool)
	for _, db := range result.Databases {
		allIDs[db.ID] = true
	}
	for _, db := range result2.Databases {
		assert.False(t, allIDs[db.ID], "page 2 should not overlap with page 1")
		allIDs[db.ID] = true
	}
	for _, db := range result3.Databases {
		assert.False(t, allIDs[db.ID], "page 3 should not overlap with page 1 or 2")
	}
}

func TestList_ExcludesSoftDeleted(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db1 := newTestDB("active-db", "platform", "default")
	err := repo.Create(ctx, db1)
	require.NoError(t, err)

	db2 := newTestDB("deleted-db", "platform", "default")
	err = repo.Create(ctx, db2)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db2.ID)
	require.NoError(t, err)

	result, err := repo.List(ctx, database.ListFilter{})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "active-db", result.Databases[0].Name)
}

func TestList_DefaultPagination(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Zero/negative values should default to page=1, limit=20
	result, err := repo.List(ctx, database.ListFilter{Page: 0, Limit: 0})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 20, result.Limit)

	result, err = repo.List(ctx, database.ListFilter{Page: -1, Limit: -5})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 20, result.Limit)
}

func TestList_MaxLimit(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()

	result, err := repo.List(ctx, database.ListFilter{Limit: 200})
	require.NoError(t, err)
	assert.Equal(t, 100, result.Limit, "limit should be capped at 100")
}

// --- Update Tests ---

func TestUpdate_OwnerTeam(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("update-owner", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	updated, err := repo.Update(ctx, db.ID, database.UpdateFields{
		OwnerTeam: strPtr("backend"),
	})
	require.NoError(t, err)

	assert.Equal(t, "backend", updated.OwnerTeam)
	assert.Equal(t, "test purpose", updated.Purpose) // unchanged
	assert.False(t, updated.UpdatedAt.IsZero(), "updated_at should be set")
}

func TestUpdate_Purpose(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("update-purpose", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	updated, err := repo.Update(ctx, db.ID, database.UpdateFields{
		Purpose: strPtr("new purpose"),
	})
	require.NoError(t, err)

	assert.Equal(t, "new purpose", updated.Purpose)
	assert.Equal(t, "platform", updated.OwnerTeam) // unchanged
}

func TestUpdate_BothFields(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("update-both", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	updated, err := repo.Update(ctx, db.ID, database.UpdateFields{
		OwnerTeam: strPtr("infra"),
		Purpose:   strPtr("analytics database"),
	})
	require.NoError(t, err)

	assert.Equal(t, "infra", updated.OwnerTeam)
	assert.Equal(t, "analytics database", updated.Purpose)
}

func TestUpdate_NoFields(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("update-noop", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	// Empty UpdateFields should return the record unchanged
	updated, err := repo.Update(ctx, db.ID, database.UpdateFields{})
	require.NoError(t, err)

	assert.Equal(t, db.ID, updated.ID)
	assert.Equal(t, "platform", updated.OwnerTeam)
}

func TestUpdate_NotFound(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.Update(ctx, uuid.New(), database.UpdateFields{
		OwnerTeam: strPtr("team"),
	})

	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestUpdate_SoftDeletedRecord(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("update-deleted", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db.ID)
	require.NoError(t, err)

	_, err = repo.Update(ctx, db.ID, database.UpdateFields{
		OwnerTeam: strPtr("team"),
	})
	assert.ErrorIs(t, err, database.ErrNotFound)
}

// --- SoftDelete Tests ---

func TestSoftDelete_Success(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("delete-me", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db.ID)
	require.NoError(t, err)

	// Verify it's no longer returned by GetByID
	_, err = repo.GetByID(ctx, db.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestSoftDelete_NotFound(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.SoftDelete(ctx, uuid.New())
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db := newTestDB("double-delete", "platform", "default")
	err := repo.Create(ctx, db)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db.ID)
	require.NoError(t, err)

	// Trying to soft-delete again should return ErrNotFound
	err = repo.SoftDelete(ctx, db.ID)
	assert.ErrorIs(t, err, database.ErrNotFound)
}

func TestSoftDelete_AllowsNameReuse(t *testing.T) {
	repo, cleanup := setupRepo(t)
	defer cleanup()

	ctx := context.Background()
	db1 := newTestDB("reusable-name", "platform", "default")
	err := repo.Create(ctx, db1)
	require.NoError(t, err)

	err = repo.SoftDelete(ctx, db1.ID)
	require.NoError(t, err)

	// Should be able to create a new record with the same name
	db2 := newTestDB("reusable-name", "platform", "default")
	err = repo.Create(ctx, db2)
	assert.NoError(t, err, "should allow name reuse after soft delete")
	assert.NotEqual(t, db1.ID, db2.ID)
}
