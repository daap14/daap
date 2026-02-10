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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	specpkg "github.com/daap14/daap/api"
	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
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

type noopManager struct{}

func (n *noopManager) ApplyCluster(_ context.Context, _ *unstructured.Unstructured) error {
	return nil
}
func (n *noopManager) ApplyPooler(_ context.Context, _ *unstructured.Unstructured) error {
	return nil
}
func (n *noopManager) DeleteCluster(_ context.Context, _, _ string) error { return nil }
func (n *noopManager) DeletePooler(_ context.Context, _, _ string) error  { return nil }
func (n *noopManager) GetClusterStatus(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
	return k8s.ClusterStatus{}, nil
}
func (n *noopManager) GetSecret(_ context.Context, _, _ string) (map[string][]byte, error) {
	return nil, nil
}

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
	router := api.NewRouter(api.RouterDeps{
		K8sChecker:  &noopHealthChecker{},
		OpenAPISpec: specpkg.OpenAPISpec,
		Repo:        &noopRepo{},
		K8sManager:  &noopManager{},
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
