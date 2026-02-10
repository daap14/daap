package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"github.com/daap14/daap/internal/team"
)

// ErrInvalidKey is returned when the provided API key does not match any active user.
var ErrInvalidKey = errors.New("invalid or revoked API key")

// Service provides authentication operations.
type Service struct {
	userRepo   UserRepository
	teamRepo   team.Repository
	bcryptCost int
}

// NewService creates a new auth Service.
func NewService(userRepo UserRepository, teamRepo team.Repository, bcryptCost int) *Service {
	return &Service{
		userRepo:   userRepo,
		teamRepo:   teamRepo,
		bcryptCost: bcryptCost,
	}
}

// GenerateKey creates a new API key. Returns the raw key, its prefix (first 8 chars),
// and the bcrypt hash. The raw key is: 32 random bytes -> base64url -> prepend "daap_".
func (s *Service) GenerateKey() (rawKey, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generating random bytes: %w", err)
	}

	rawKey = "daap_" + base64.RawURLEncoding.EncodeToString(b)
	prefix = rawKey[:8]

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(rawKey), s.bcryptCost)
	if err != nil {
		return "", "", "", fmt.Errorf("hashing key: %w", err)
	}
	hash = string(hashBytes)

	return rawKey, prefix, hash, nil
}

// Authenticate resolves a raw API key to an Identity. It extracts the prefix,
// looks up candidates, and bcrypt-compares each one.
func (s *Service) Authenticate(ctx context.Context, rawKey string) (*Identity, error) {
	if len(rawKey) < 8 {
		return nil, ErrInvalidKey
	}

	prefix := rawKey[:8]

	candidates, err := s.userRepo.FindByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("finding users by prefix: %w", err)
	}

	for _, u := range candidates {
		if bcrypt.CompareHashAndPassword([]byte(u.ApiKeyHash), []byte(rawKey)) == nil {
			return s.buildIdentity(ctx, &u)
		}
	}

	return nil, ErrInvalidKey
}

// BootstrapSuperuser creates the initial superuser if the users table is empty.
// Returns the raw API key (only displayed once). If users already exist, returns empty string.
func (s *Service) BootstrapSuperuser(ctx context.Context) (string, error) {
	count, err := s.userRepo.CountAll(ctx)
	if err != nil {
		return "", fmt.Errorf("counting users: %w", err)
	}

	if count > 0 {
		return "", nil
	}

	rawKey, prefix, hash, err := s.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generating superuser key: %w", err)
	}

	user := &User{
		Name:         "superuser",
		TeamID:       nil,
		IsSuperuser:  true,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return "", fmt.Errorf("creating superuser: %w", err)
	}

	slog.Info("Superuser API key created", "key", rawKey)

	return rawKey, nil
}

// buildIdentity constructs an Identity from a User, fetching team info if applicable.
func (s *Service) buildIdentity(ctx context.Context, u *User) (*Identity, error) {
	identity := &Identity{
		UserID:      u.ID,
		UserName:    u.Name,
		TeamID:      u.TeamID,
		IsSuperuser: u.IsSuperuser,
	}

	if u.TeamID != nil {
		t, err := s.teamRepo.GetByID(ctx, *u.TeamID)
		if err != nil {
			return nil, fmt.Errorf("fetching team for identity: %w", err)
		}
		identity.TeamName = &t.Name
		identity.Role = &t.Role
	}

	return identity, nil
}
