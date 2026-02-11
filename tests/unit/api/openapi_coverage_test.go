package api_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	specpkg "github.com/daap14/daap/api"
	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/team"
	"github.com/daap14/daap/internal/tier"
)

// openAPISpec is the minimal structure needed to extract paths from the spec.
type openAPISpec struct {
	Paths map[string]map[string]interface{} `json:"paths"`
}

// --- Noop implementations to satisfy RouterDeps interfaces ---

type noopHealthChecker struct{}

func (n *noopHealthChecker) CheckConnectivity(_ context.Context) k8s.ConnectivityStatus {
	return k8s.ConnectivityStatus{Connected: false}
}

type noopRepo struct{}

func (n *noopRepo) Create(_ context.Context, _ *database.Database) error { return nil }
func (n *noopRepo) GetByID(_ context.Context, _ uuid.UUID) (*database.Database, error) {
	return nil, nil
}
func (n *noopRepo) List(_ context.Context, _ database.ListFilter) (*database.ListResult, error) {
	return nil, nil
}
func (n *noopRepo) Update(_ context.Context, _ uuid.UUID, _ database.UpdateFields) (*database.Database, error) {
	return nil, nil
}
func (n *noopRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ database.StatusUpdate) (*database.Database, error) {
	return nil, nil
}
func (n *noopRepo) SoftDelete(_ context.Context, _ uuid.UUID) error { return nil }

type noopBlueprintRepo struct{}

func (n *noopBlueprintRepo) Create(_ context.Context, _ *blueprint.Blueprint) error { return nil }
func (n *noopBlueprintRepo) GetByID(_ context.Context, _ uuid.UUID) (*blueprint.Blueprint, error) {
	return nil, nil
}
func (n *noopBlueprintRepo) GetByName(_ context.Context, _ string) (*blueprint.Blueprint, error) {
	return nil, nil
}
func (n *noopBlueprintRepo) List(_ context.Context) ([]blueprint.Blueprint, error) { return nil, nil }
func (n *noopBlueprintRepo) Delete(_ context.Context, _ uuid.UUID) error            { return nil }

type noopTeamRepo struct{}

func (n *noopTeamRepo) Create(_ context.Context, _ *team.Team) error               { return nil }
func (n *noopTeamRepo) GetByID(_ context.Context, _ uuid.UUID) (*team.Team, error) { return nil, nil }
func (n *noopTeamRepo) GetByName(_ context.Context, _ string) (*team.Team, error)  { return nil, nil }
func (n *noopTeamRepo) List(_ context.Context) ([]team.Team, error)                { return nil, nil }
func (n *noopTeamRepo) Delete(_ context.Context, _ uuid.UUID) error                { return nil }

type noopTierRepo struct{}

func (n *noopTierRepo) Create(_ context.Context, _ *tier.Tier) error               { return nil }
func (n *noopTierRepo) GetByID(_ context.Context, _ uuid.UUID) (*tier.Tier, error) { return nil, nil }
func (n *noopTierRepo) GetByName(_ context.Context, _ string) (*tier.Tier, error)  { return nil, nil }
func (n *noopTierRepo) List(_ context.Context) ([]tier.Tier, error)                { return nil, nil }
func (n *noopTierRepo) Update(_ context.Context, _ uuid.UUID, _ tier.UpdateFields) (*tier.Tier, error) {
	return nil, nil
}
func (n *noopTierRepo) Delete(_ context.Context, _ uuid.UUID) error { return nil }

type noopUserRepo struct{}

func (n *noopUserRepo) Create(_ context.Context, _ *auth.User) error               { return nil }
func (n *noopUserRepo) GetByID(_ context.Context, _ uuid.UUID) (*auth.User, error) { return nil, nil }
func (n *noopUserRepo) FindByPrefix(_ context.Context, _ string) ([]auth.User, error) {
	return nil, nil
}
func (n *noopUserRepo) List(_ context.Context) ([]auth.User, error) { return nil, nil }
func (n *noopUserRepo) Revoke(_ context.Context, _ uuid.UUID) error { return nil }
func (n *noopUserRepo) CountAll(_ context.Context) (int, error)     { return 0, nil }

// --- Test ---

func TestOpenAPISpec_RoutesCoverAllPaths(t *testing.T) {
	t.Parallel()

	// Parse spec paths from the embedded YAML
	specJSON, err := yaml.YAMLToJSON(specpkg.OpenAPISpec)
	require.NoError(t, err, "embedded spec must convert to JSON")

	var spec openAPISpec
	err = yaml.Unmarshal(specJSON, &spec)
	require.NoError(t, err, "spec JSON must unmarshal")

	specRoutes := extractSpecRoutes(t, spec)
	require.NotEmpty(t, specRoutes, "spec should define at least one route")

	// Build a router with noop deps so all routes are registered
	teamRepo := &noopTeamRepo{}
	userRepo := &noopUserRepo{}
	authService := auth.NewService(userRepo, teamRepo, 4)

	router := api.NewRouter(api.RouterDeps{
		K8sChecker:    &noopHealthChecker{},
		OpenAPISpec:   specpkg.OpenAPISpec,
		Repo:          &noopRepo{},
		AuthService:   authService,
		TeamRepo:      teamRepo,
		TierRepo:      &noopTierRepo{},
		BlueprintRepo: &noopBlueprintRepo{},
		UserRepo:      userRepo,
	})

	chiRoutes := extractChiRoutes(t, router)
	require.NotEmpty(t, chiRoutes, "Chi router should have at least one route")

	// Every spec path+method must have a matching Chi route
	for _, sr := range specRoutes {
		t.Run(fmt.Sprintf("spec_%s_%s_has_Chi_route", sr.method, sr.path), func(t *testing.T) {
			found := false
			for _, cr := range chiRoutes {
				if cr.method == sr.method && cr.path == sr.path {
					found = true
					break
				}
			}
			assert.True(t, found, "spec route %s %s not found in Chi router", sr.method, sr.path)
		})
	}

	// Every Chi route must have a matching spec path+method
	for _, cr := range chiRoutes {
		t.Run(fmt.Sprintf("Chi_%s_%s_has_spec_path", cr.method, cr.path), func(t *testing.T) {
			found := false
			for _, sr := range specRoutes {
				if sr.method == cr.method && sr.path == cr.path {
					found = true
					break
				}
			}
			assert.True(t, found, "Chi route %s %s not found in OpenAPI spec", cr.method, cr.path)
		})
	}
}

type route struct {
	method string
	path   string
}

func extractSpecRoutes(t *testing.T, spec openAPISpec) []route {
	t.Helper()
	var routes []route
	for path, methods := range spec.Paths {
		for method := range methods {
			routes = append(routes, route{
				method: strings.ToUpper(method),
				path:   path,
			})
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].path == routes[j].path {
			return routes[i].method < routes[j].method
		}
		return routes[i].path < routes[j].path
	})
	return routes
}

func extractChiRoutes(t *testing.T, r *chi.Mux) []route {
	t.Helper()
	var routes []route
	walkFunc := func(method, routePath string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Normalize: Chi subroutes produce trailing slashes (e.g. /databases/)
		// while OpenAPI uses /databases — strip trailing slash for comparison.
		normalized := strings.TrimRight(routePath, "/")
		if normalized == "" {
			normalized = "/"
		}
		routes = append(routes, route{
			method: method,
			path:   normalized,
		})
		return nil
	}
	err := chi.Walk(r, walkFunc)
	require.NoError(t, err, "chi.Walk should not error")

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].path == routes[j].path {
			return routes[i].method < routes[j].method
		}
		return routes[i].path < routes[j].path
	})
	return routes
}
