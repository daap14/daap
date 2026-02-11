package tier_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/tier"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupTierRepo(t *testing.T) (tier.Repository, *pgxpool.Pool, func()) {
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

	// Clean slate: truncate databases first (FK dependency), then tiers
	_, err = pool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE tiers CASCADE")
	require.NoError(t, err)

	repo := tier.NewPostgresRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return repo, pool, cleanup
}

func newTestTier(name string) *tier.Tier {
	return &tier.Tier{
		Name:                name,
		Description:         "Test tier",
		Instances:           1,
		CPU:                 "500m",
		Memory:              "512Mi",
		StorageSize:         "1Gi",
		StorageClass:        "",
		PGVersion:           "16",
		PoolMode:            "transaction",
		MaxConnections:      100,
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("standard")

	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, tr.ID)
	assert.Equal(t, "standard", tr.Name)
	assert.Equal(t, "Test tier", tr.Description)
	assert.Equal(t, 1, tr.Instances)
	assert.Equal(t, "500m", tr.CPU)
	assert.Equal(t, "512Mi", tr.Memory)
	assert.Equal(t, "1Gi", tr.StorageSize)
	assert.Equal(t, "", tr.StorageClass)
	assert.Equal(t, "16", tr.PGVersion)
	assert.Equal(t, "transaction", tr.PoolMode)
	assert.Equal(t, 100, tr.MaxConnections)
	assert.Equal(t, "hard_delete", tr.DestructionStrategy)
	assert.False(t, tr.BackupEnabled)
	assert.False(t, tr.CreatedAt.IsZero())
	assert.False(t, tr.UpdatedAt.IsZero())
}

func TestCreate_DuplicateName(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr1 := newTestTier("duptier")
	err := repo.Create(ctx, tr1)
	require.NoError(t, err)

	tr2 := newTestTier("duptier")
	err = repo.Create(ctx, tr2)
	assert.ErrorIs(t, err, tier.ErrDuplicateTierName)
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("get-by-id")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, tr.ID)
	require.NoError(t, err)

	assert.Equal(t, tr.ID, found.ID)
	assert.Equal(t, "get-by-id", found.Name)
	assert.Equal(t, 1, found.Instances)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

// --- GetByName Tests ---

func TestGetByName_Success(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("by-name")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	found, err := repo.GetByName(ctx, "by-name")
	require.NoError(t, err)

	assert.Equal(t, tr.ID, found.ID)
	assert.Equal(t, "by-name", found.Name)
}

func TestGetByName_NotFound(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByName(ctx, "nonexistent")
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

// --- List Tests ---

func TestList_Empty(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tiers, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Empty(t, tiers)
}

func TestList_MultipleTiers(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		tr := newTestTier(name)
		err := repo.Create(ctx, tr)
		require.NoError(t, err)
	}

	tiers, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Len(t, tiers, 3)
	// Ordered by created_at ASC
	assert.Equal(t, "alpha", tiers[0].Name)
	assert.Equal(t, "beta", tiers[1].Name)
	assert.Equal(t, "gamma", tiers[2].Name)
}

// --- Update Tests ---

func TestUpdate_Success(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("updatable")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	newDesc := "Updated description"
	newInstances := 3
	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{
		Description: &newDesc,
		Instances:   &newInstances,
	})
	require.NoError(t, err)

	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, 3, updated.Instances)
	assert.Equal(t, "updatable", updated.Name) // name unchanged
	assert.True(t, updated.UpdatedAt.After(tr.UpdatedAt) || updated.UpdatedAt.Equal(tr.UpdatedAt))
}

func TestUpdate_NotFound(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	newDesc := "nope"
	_, err := repo.Update(ctx, uuid.New(), tier.UpdateFields{
		Description: &newDesc,
	})
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestUpdate_PartialFields(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("partial")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	newCPU := "1"
	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{
		CPU: &newCPU,
	})
	require.NoError(t, err)

	assert.Equal(t, "1", updated.CPU)
	assert.Equal(t, "512Mi", updated.Memory) // unchanged
	assert.Equal(t, 1, updated.Instances)    // unchanged
}

func TestUpdate_NoFields(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("no-change")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{})
	require.NoError(t, err)

	assert.Equal(t, tr.ID, updated.ID)
	assert.Equal(t, "no-change", updated.Name)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("deleteme")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	err = repo.Delete(ctx, tr.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, tr.ID)
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	repo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestDelete_TierHasDatabases(t *testing.T) {
	repo, pool, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("has-dbs")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	// Create a team for the database's owner_team_id FK
	var teamID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO teams (name, role) VALUES ($1, $2) RETURNING id`,
		"testteam", "platform",
	).Scan(&teamID)
	require.NoError(t, err)

	// Insert a database referencing this tier directly via SQL
	_, err = pool.Exec(ctx,
		`INSERT INTO databases (name, owner_team_id, tier_id, purpose, namespace, cluster_name, pooler_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"testdb", teamID, tr.ID, "test", "default", "daap-testdb", "daap-testdb-pooler", "provisioning",
	)
	require.NoError(t, err)

	err = repo.Delete(ctx, tr.ID)
	assert.ErrorIs(t, err, tier.ErrTierHasDatabases)
}

func TestDelete_SoftDeletedDatabaseDoesNotBlock(t *testing.T) {
	repo, pool, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("soft-del")
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	// Create a team for the database's owner_team_id FK
	var teamID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO teams (name, role) VALUES ($1, $2) RETURNING id`,
		"softdelteam", "platform",
	).Scan(&teamID)
	require.NoError(t, err)

	// Insert a database referencing this tier, then soft-delete it
	_, err = pool.Exec(ctx,
		`INSERT INTO databases (name, owner_team_id, tier_id, purpose, namespace, cluster_name, pooler_name, status, deleted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
		"softdb", teamID, tr.ID, "test", "default", "daap-softdb", "daap-softdb-pooler", "deleted",
	)
	require.NoError(t, err)

	// Tier delete should succeed — only active databases block deletion
	err = repo.Delete(ctx, tr.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, tr.ID)
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}
