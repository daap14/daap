package blueprint_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/blueprint"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupBlueprintRepo(t *testing.T) (blueprint.Repository, *pgxpool.Pool, func()) {
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

	// Clean slate: truncate tiers first (FK dependency on blueprints), then blueprints
	_, err = pool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE tiers CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE blueprints CASCADE")
	require.NoError(t, err)

	repo := blueprint.NewPostgresRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return repo, pool, cleanup
}

func newTestBlueprint(name string) *blueprint.Blueprint {
	return &blueprint.Blueprint{
		Name:      name,
		Provider:  "cnpg",
		Manifests: "---\napiVersion: postgresql.cnpg.io/v1\nkind: Cluster\nmetadata:\n  name: daap-{{ .Name }}\n  namespace: {{ .Namespace }}\nspec:\n  instances: 1\n",
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := newTestBlueprint("cnpg-dev")

	err := repo.Create(ctx, bp)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, bp.ID)
	assert.Equal(t, "cnpg-dev", bp.Name)
	assert.Equal(t, "cnpg", bp.Provider)
	assert.Contains(t, bp.Manifests, "apiVersion: postgresql.cnpg.io/v1")
	assert.False(t, bp.CreatedAt.IsZero())
	assert.False(t, bp.UpdatedAt.IsZero())
}

func TestCreate_DuplicateName(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp1 := newTestBlueprint("dup-bp")
	err := repo.Create(ctx, bp1)
	require.NoError(t, err)

	bp2 := newTestBlueprint("dup-bp")
	err = repo.Create(ctx, bp2)
	assert.ErrorIs(t, err, blueprint.ErrDuplicateBlueprintName)
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := newTestBlueprint("get-by-id")
	err := repo.Create(ctx, bp)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, bp.ID)
	require.NoError(t, err)

	assert.Equal(t, bp.ID, found.ID)
	assert.Equal(t, "get-by-id", found.Name)
	assert.Equal(t, "cnpg", found.Provider)
	assert.Equal(t, bp.Manifests, found.Manifests)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, blueprint.ErrBlueprintNotFound)
}

// --- GetByName Tests ---

func TestGetByName_Success(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := newTestBlueprint("by-name")
	err := repo.Create(ctx, bp)
	require.NoError(t, err)

	found, err := repo.GetByName(ctx, "by-name")
	require.NoError(t, err)

	assert.Equal(t, bp.ID, found.ID)
	assert.Equal(t, "by-name", found.Name)
	assert.Equal(t, "cnpg", found.Provider)
}

func TestGetByName_NotFound(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByName(ctx, "nonexistent")
	assert.ErrorIs(t, err, blueprint.ErrBlueprintNotFound)
}

// --- List Tests ---

func TestList_Empty(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	blueprints, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Empty(t, blueprints)
}

func TestList_MultipleBlueprints(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		bp := newTestBlueprint(name)
		err := repo.Create(ctx, bp)
		require.NoError(t, err)
	}

	blueprints, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Len(t, blueprints, 3)
	// Ordered by created_at ASC
	assert.Equal(t, "alpha", blueprints[0].Name)
	assert.Equal(t, "beta", blueprints[1].Name)
	assert.Equal(t, "gamma", blueprints[2].Name)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := newTestBlueprint("deleteme")
	err := repo.Create(ctx, bp)
	require.NoError(t, err)

	err = repo.Delete(ctx, bp.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, bp.ID)
	assert.ErrorIs(t, err, blueprint.ErrBlueprintNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	repo, _, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, blueprint.ErrBlueprintNotFound)
}

func TestDelete_BlueprintHasTiers(t *testing.T) {
	repo, pool, cleanup := setupBlueprintRepo(t)
	defer cleanup()

	ctx := context.Background()
	bp := newTestBlueprint("has-tiers")
	err := repo.Create(ctx, bp)
	require.NoError(t, err)

	// Insert a tier referencing this blueprint directly via SQL
	_, err = pool.Exec(ctx,
		`INSERT INTO tiers (name, description, blueprint_id, destruction_strategy, backup_enabled)
		 VALUES ($1, $2, $3, $4, $5)`,
		"test-tier", "a tier", bp.ID, "hard_delete", false,
	)
	require.NoError(t, err)

	err = repo.Delete(ctx, bp.ID)
	assert.ErrorIs(t, err, blueprint.ErrBlueprintHasTiers)
}
