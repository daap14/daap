package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/team"
)

const testBcryptCost = 4 // low cost for fast tests

func setupService(t *testing.T) (*auth.Service, auth.UserRepository, team.Repository, *pgxpool.Pool, func()) {
	t.Helper()

	userRepo, pool, cleanup := setupUserRepo(t)
	teamRepo := team.NewRepository(pool)
	svc := auth.NewService(userRepo, teamRepo, testBcryptCost)

	return svc, userRepo, teamRepo, pool, cleanup
}

// --- GenerateKey Tests ---

func TestGenerateKey_Format(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(rawKey, "daap_"), "raw key should start with daap_")
	assert.Len(t, prefix, 8, "prefix should be 8 characters")
	assert.Equal(t, rawKey[:8], prefix, "prefix should be first 8 chars of raw key")
	assert.NotEmpty(t, hash, "hash should not be empty")

	// Verify bcrypt hash is valid
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(rawKey))
	assert.NoError(t, err, "hash should verify against raw key")
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	key1, _, _, err := svc.GenerateKey()
	require.NoError(t, err)

	key2, _, _, err := svc.GenerateKey()
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2, "generated keys should be unique")
}

// --- Authenticate Tests ---

func TestAuthenticate_ValidKey(t *testing.T) {
	svc, userRepo, teamRepo, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	// Create a team and user
	tm := &team.Team{Name: "authteam", Role: "platform"}
	err := teamRepo.Create(ctx, tm)
	require.NoError(t, err)

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "authuser",
		TeamID:       &tm.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	identity, err := svc.Authenticate(ctx, rawKey)
	require.NoError(t, err)

	assert.Equal(t, u.ID, identity.UserID)
	assert.Equal(t, "authuser", identity.UserName)
	assert.Equal(t, &tm.ID, identity.TeamID)
	assert.Equal(t, "authteam", *identity.TeamName)
	assert.Equal(t, "platform", *identity.Role)
	assert.False(t, identity.IsSuperuser)
}

func TestAuthenticate_SuperuserKey(t *testing.T) {
	svc, userRepo, _, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "superuser",
		TeamID:       nil,
		IsSuperuser:  true,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	identity, err := svc.Authenticate(ctx, rawKey)
	require.NoError(t, err)

	assert.True(t, identity.IsSuperuser)
	assert.Nil(t, identity.TeamID)
	assert.Nil(t, identity.TeamName)
	assert.Nil(t, identity.Role)
}

func TestAuthenticate_InvalidKey(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := svc.Authenticate(ctx, "daap_invalidkeyvalue12345678901234567890")
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

func TestAuthenticate_TooShortKey(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := svc.Authenticate(ctx, "short")
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

func TestAuthenticate_RevokedUser(t *testing.T) {
	svc, userRepo, teamRepo, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	tm := &team.Team{Name: "revoketeam", Role: "product"}
	err := teamRepo.Create(ctx, tm)
	require.NoError(t, err)

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "revokeduser",
		TeamID:       &tm.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	err = userRepo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	_, err = svc.Authenticate(ctx, rawKey)
	assert.ErrorIs(t, err, auth.ErrInvalidKey)
}

// --- BootstrapSuperuser Tests ---

func TestBootstrapSuperuser_EmptyTable(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	rawKey, err := svc.BootstrapSuperuser(ctx)
	require.NoError(t, err)

	assert.NotEmpty(t, rawKey)
	assert.True(t, strings.HasPrefix(rawKey, "daap_"))

	// Verify the superuser can authenticate
	identity, err := svc.Authenticate(ctx, rawKey)
	require.NoError(t, err)
	assert.True(t, identity.IsSuperuser)
	assert.Equal(t, "superuser", identity.UserName)
}

func TestBootstrapSuperuser_NonEmptyTable(t *testing.T) {
	svc, userRepo, teamRepo, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	// Create a team and user first
	tm := &team.Team{Name: "existing", Role: "platform"}
	err := teamRepo.Create(ctx, tm)
	require.NoError(t, err)

	_, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "existing-user",
		TeamID:       &tm.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	// Bootstrap should be a no-op
	rawKey, err := svc.BootstrapSuperuser(ctx)
	require.NoError(t, err)
	assert.Empty(t, rawKey, "should return empty key when users already exist")
}

func TestBootstrapSuperuser_Idempotent(t *testing.T) {
	svc, _, _, _, cleanup := setupService(t)
	defer cleanup()

	ctx := context.Background()

	key1, err := svc.BootstrapSuperuser(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, key1)

	// Second call should be a no-op
	key2, err := svc.BootstrapSuperuser(ctx)
	require.NoError(t, err)
	assert.Empty(t, key2, "second bootstrap should return empty key")
}
