package auth_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/auth"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

func setupUserRepo(t *testing.T) (auth.UserRepository, *pgxpool.Pool, func()) {
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

	// Clean slate
	_, err = pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	repo := auth.NewRepository(pool)
	cleanup := func() {
		pool.Close()
	}
	return repo, pool, cleanup
}

// createTestTeam inserts a team directly and returns its ID.
func createTestTeam(t *testing.T, pool *pgxpool.Pool, name, role string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO teams (name, role) VALUES ($1, $2) RETURNING id`, name, role,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func newTestUser(name string, teamID *uuid.UUID, isSuperuser bool) *auth.User {
	return &auth.User{
		Name:         name,
		TeamID:       teamID,
		IsSuperuser:  isSuperuser,
		ApiKeyPrefix: "daap_tes",
		ApiKeyHash:   "$2a$04$abcdefghijklmnopqrstuuAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
}

// --- Create Tests ---

func TestCreate_Success(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "ops", "platform")
	u := newTestUser("alice", &teamID, false)

	err := repo.Create(ctx, u)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.False(t, u.CreatedAt.IsZero())
}

func TestCreate_Superuser(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	u := newTestUser("superuser", nil, true)

	err := repo.Create(ctx, u)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.True(t, u.IsSuperuser)
	assert.Nil(t, u.TeamID)
}

func TestCreate_DuplicateSuperuser(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	u1 := newTestUser("superuser1", nil, true)
	u1.ApiKeyPrefix = "daap_su1"
	err := repo.Create(ctx, u1)
	require.NoError(t, err)

	u2 := newTestUser("superuser2", nil, true)
	u2.ApiKeyPrefix = "daap_su2"
	err = repo.Create(ctx, u2)
	assert.Error(t, err, "partial unique index should prevent second superuser")
}

// --- GetByID Tests ---

func TestGetByID_Success(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "backend", "platform")
	u := newTestUser("bob", &teamID, false)
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)

	assert.Equal(t, u.ID, found.ID)
	assert.Equal(t, "bob", found.Name)
	assert.Equal(t, &teamID, found.TeamID)
	assert.False(t, found.IsSuperuser)
	assert.Nil(t, found.RevokedAt)
}

func TestGetByID_NotFound(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

// --- FindByPrefix Tests ---

func TestFindByPrefix_ReturnsActiveUsers(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "devs", "product")

	u := &auth.User{
		Name:         "charlie",
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: "daap_abc",
		ApiKeyHash:   "$2a$04$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	users, err := repo.FindByPrefix(ctx, "daap_abc")
	require.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Equal(t, "charlie", users[0].Name)
}

func TestFindByPrefix_ExcludesRevoked(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "testers", "product")

	u := &auth.User{
		Name:         "revoked-user",
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: "daap_rev",
		ApiKeyHash:   "$2a$04$yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy",
	}
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	err = repo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	users, err := repo.FindByPrefix(ctx, "daap_rev")
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestFindByPrefix_NoMatch(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	users, err := repo.FindByPrefix(ctx, "daap_zzz")
	require.NoError(t, err)
	assert.Empty(t, users)
}

// --- List Tests ---

func TestList_Empty(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	users, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, users)
}

func TestList_ReturnsAll(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "allteam", "platform")

	for i, name := range []string{"user-a", "user-b"} {
		u := &auth.User{
			Name:         name,
			TeamID:       &teamID,
			IsSuperuser:  false,
			ApiKeyPrefix: "daap_l" + string(rune('a'+i)),
			ApiKeyHash:   "$2a$04$zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz" + string(rune('a'+i)),
		}
		err := repo.Create(ctx, u)
		require.NoError(t, err)
	}

	users, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

// --- Revoke Tests ---

func TestRevoke_Success(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "revteam", "platform")
	u := &auth.User{
		Name:         "revocable",
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: "daap_rvk",
		ApiKeyHash:   "$2a$04$rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr",
	}
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	err = repo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.NotNil(t, found.RevokedAt)
}

func TestRevoke_AlreadyRevoked(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "revteam2", "product")
	u := &auth.User{
		Name:         "double-revoke",
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: "daap_drv",
		ApiKeyHash:   "$2a$04$sssssssssssssssssssssssssssssssssssssssssssssssss",
	}
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	err = repo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	err = repo.Revoke(ctx, u.ID)
	assert.ErrorIs(t, err, auth.ErrUserRevoked)
}

func TestRevoke_NotFound(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Revoke(ctx, uuid.New())
	assert.ErrorIs(t, err, auth.ErrUserNotFound)
}

// --- CountAll Tests ---

func TestCountAll_Empty(t *testing.T) {
	repo, _, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	count, err := repo.CountAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCountAll_IncludesRevoked(t *testing.T) {
	repo, pool, cleanup := setupUserRepo(t)
	defer cleanup()

	ctx := context.Background()
	teamID := createTestTeam(t, pool, "countteam", "platform")

	u := &auth.User{
		Name:         "counted",
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: "daap_cnt",
		ApiKeyHash:   "$2a$04$ttttttttttttttttttttttttttttttttttttttttttttttttt",
	}
	err := repo.Create(ctx, u)
	require.NoError(t, err)

	err = repo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	count, err := repo.CountAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "CountAll should include revoked users")
}
