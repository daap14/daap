package tier_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/tier"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupTierRepo(t *testing.T) (tier.Repository, blueprint.Repository, *pgxpool.Pool, func()) {
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

	// Clean slate: truncate databases first (FK dependency), then tiers, then blueprints
	_, err = pool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE tiers CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE blueprints CASCADE")
	require.NoError(t, err)

	tierRepo := tier.NewPostgresRepository(pool)
	bpRepo := blueprint.NewPostgresRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return tierRepo, bpRepo, pool, cleanup
}

func createTestBlueprint(t *testing.T, bpRepo blueprint.Repository, name string) *blueprint.Blueprint {
	t.Helper()
	bp := &blueprint.Blueprint{
		Name:      name,
		Provider:  "cnpg",
		Manifests: "---\napiVersion: postgresql.cnpg.io/v1\nkind: Cluster\nmetadata:\n  name: test\n",
	}
	err := bpRepo.Create(context.Background(), bp)
	require.NoError(t, err)
	return bp
}

func newTestTier(name string, blueprintID *uuid.UUID) *tier.Tier {
	return &tier.Tier{
		Name:                name,
		Description:         "Test tier",
		BlueprintID:         blueprintID,
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-create")
	tr := newTestTier("standard", &bp.ID)

	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, tr.ID)
	assert.Equal(t, "standard", tr.Name)
	assert.Equal(t, "Test tier", tr.Description)
	assert.Equal(t, &bp.ID, tr.BlueprintID)
	assert.Equal(t, "bp-create", tr.BlueprintName)
	assert.Equal(t, "hard_delete", tr.DestructionStrategy)
	assert.False(t, tr.BackupEnabled)
	assert.False(t, tr.CreatedAt.IsZero())
	assert.False(t, tr.UpdatedAt.IsZero())
}

func TestCreate_NilBlueprintID(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tr := newTestTier("no-blueprint", nil)

	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, tr.ID)
	assert.Nil(t, tr.BlueprintID)
	assert.Equal(t, "", tr.BlueprintName)
}

func TestCreate_DuplicateName(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-dup")
	tr1 := newTestTier("duptier", &bp.ID)
	err := repo.Create(ctx, tr1)
	require.NoError(t, err)

	tr2 := newTestTier("duptier", &bp.ID)
	err = repo.Create(ctx, tr2)
	assert.ErrorIs(t, err, tier.ErrDuplicateTierName)
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-getbyid")
	tr := newTestTier("get-by-id", &bp.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, tr.ID)
	require.NoError(t, err)

	assert.Equal(t, tr.ID, found.ID)
	assert.Equal(t, "get-by-id", found.Name)
	assert.Equal(t, &bp.ID, found.BlueprintID)
	assert.Equal(t, "bp-getbyid", found.BlueprintName)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

// --- GetByName Tests ---

func TestGetByName_Success(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-byname")
	tr := newTestTier("by-name", &bp.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	found, err := repo.GetByName(ctx, "by-name")
	require.NoError(t, err)

	assert.Equal(t, tr.ID, found.ID)
	assert.Equal(t, "by-name", found.Name)
	assert.Equal(t, "bp-byname", found.BlueprintName)
}

func TestGetByName_NotFound(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByName(ctx, "nonexistent")
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

// --- List Tests ---

func TestList_Empty(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	tiers, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Empty(t, tiers)
}

func TestList_MultipleTiers(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-list")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		tr := newTestTier(name, &bp.ID)
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
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-update")
	tr := newTestTier("updatable", &bp.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	newDesc := "Updated description"
	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{
		Description: &newDesc,
	})
	require.NoError(t, err)

	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, "updatable", updated.Name) // name unchanged
	assert.True(t, updated.UpdatedAt.After(tr.UpdatedAt) || updated.UpdatedAt.Equal(tr.UpdatedAt))
}

func TestUpdate_BlueprintID(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp1 := createTestBlueprint(t, bpRepo, "bp-update-1")
	bp2 := createTestBlueprint(t, bpRepo, "bp-update-2")
	tr := newTestTier("update-bp", &bp1.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	assert.Equal(t, "bp-update-1", tr.BlueprintName)

	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{
		BlueprintID: &bp2.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, &bp2.ID, updated.BlueprintID)
	assert.Equal(t, "bp-update-2", updated.BlueprintName)
}

func TestUpdate_NotFound(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	newDesc := "nope"
	_, err := repo.Update(ctx, uuid.New(), tier.UpdateFields{
		Description: &newDesc,
	})
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestUpdate_NoFields(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-nochange")
	tr := newTestTier("no-change", &bp.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	updated, err := repo.Update(ctx, tr.ID, tier.UpdateFields{})
	require.NoError(t, err)

	assert.Equal(t, tr.ID, updated.ID)
	assert.Equal(t, "no-change", updated.Name)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	repo, bpRepo, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-delete")
	tr := newTestTier("deleteme", &bp.ID)
	err := repo.Create(ctx, tr)
	require.NoError(t, err)

	err = repo.Delete(ctx, tr.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, tr.ID)
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	repo, _, _, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, tier.ErrTierNotFound)
}

func TestDelete_TierHasDatabases(t *testing.T) {
	repo, bpRepo, pool, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-has-dbs")
	tr := newTestTier("has-dbs", &bp.ID)
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
	repo, bpRepo, pool, cleanup := setupTierRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := createTestBlueprint(t, bpRepo, "bp-soft-del")
	tr := newTestTier("soft-del", &bp.ID)
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
