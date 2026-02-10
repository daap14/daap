package team_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/team"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupTeamRepo(t *testing.T) (team.Repository, *pgxpool.Pool, func()) {
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

	// Clean slate: truncate users first (FK dependency), then teams
	_, err = pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	repo := team.NewRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return repo, pool, cleanup
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "ops", Role: "platform"}

	err := repo.Create(ctx, tm)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, tm.ID)
	assert.Equal(t, "ops", tm.Name)
	assert.Equal(t, "platform", tm.Role)
	assert.False(t, tm.CreatedAt.IsZero())
	assert.False(t, tm.UpdatedAt.IsZero())
}

func TestCreate_ProductRole(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "frontend", Role: "product"}

	err := repo.Create(ctx, tm)
	require.NoError(t, err)

	assert.Equal(t, "product", tm.Role)
}

func TestCreate_DuplicateName(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm1 := &team.Team{Name: "dupteam", Role: "platform"}
	err := repo.Create(ctx, tm1)
	require.NoError(t, err)

	tm2 := &team.Team{Name: "dupteam", Role: "product"}
	err = repo.Create(ctx, tm2)
	assert.ErrorIs(t, err, team.ErrDuplicateTeamName)
}

func TestCreate_InvalidRole(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "badteam", Role: "admin"}

	err := repo.Create(ctx, tm)
	assert.Error(t, err, "invalid role should be rejected by CHECK constraint")
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "getteam", Role: "platform"}
	err := repo.Create(ctx, tm)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, tm.ID)
	require.NoError(t, err)

	assert.Equal(t, tm.ID, found.ID)
	assert.Equal(t, "getteam", found.Name)
	assert.Equal(t, "platform", found.Role)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, team.ErrTeamNotFound)
}

// --- List Tests ---

func TestList_Empty(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	teams, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Empty(t, teams)
}

func TestList_ReturnsAll(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		tm := &team.Team{Name: name, Role: "platform"}
		err := repo.Create(ctx, tm)
		require.NoError(t, err)
	}

	teams, err := repo.List(ctx)
	require.NoError(t, err)

	assert.Len(t, teams, 3)
}

func TestList_OrderedByCreatedAt(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	names := []string{"first", "second", "third"}
	for _, name := range names {
		tm := &team.Team{Name: name, Role: "platform"}
		err := repo.Create(ctx, tm)
		require.NoError(t, err)
	}

	teams, err := repo.List(ctx)
	require.NoError(t, err)

	require.Len(t, teams, 3)
	assert.Equal(t, "first", teams[0].Name)
	assert.Equal(t, "second", teams[1].Name)
	assert.Equal(t, "third", teams[2].Name)
}

// --- Delete Tests ---

func TestDelete_Success(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "deleteme", Role: "platform"}
	err := repo.Create(ctx, tm)
	require.NoError(t, err)

	err = repo.Delete(ctx, tm.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, tm.ID)
	assert.ErrorIs(t, err, team.ErrTeamNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	repo, _, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, team.ErrTeamNotFound)
}

func TestDelete_TeamHasUsers(t *testing.T) {
	repo, pool, cleanup := setupTeamRepo(t)
	defer cleanup()

	ctx := context.Background()
	tm := &team.Team{Name: "hasusers", Role: "platform"}
	err := repo.Create(ctx, tm)
	require.NoError(t, err)

	// Insert a user referencing this team directly via SQL
	_, err = pool.Exec(ctx,
		`INSERT INTO users (name, team_id, is_superuser, api_key_prefix, api_key_hash)
		 VALUES ($1, $2, FALSE, $3, $4)`,
		"testuser", tm.ID, "daap_tes", "$2a$04$fakehash")
	require.NoError(t, err)

	err = repo.Delete(ctx, tm.ID)
	assert.ErrorIs(t, err, team.ErrTeamHasUsers)
}
