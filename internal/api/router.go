package api

import (
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/team"
	"github.com/daap14/daap/internal/tier"
)

// RouterDeps holds all dependencies needed by the router.
type RouterDeps struct {
	K8sChecker    k8s.HealthChecker
	DBPinger      handler.DBPinger
	Version       string
	Repo          database.Repository
	K8sManager    k8s.ResourceManager
	Namespace     string
	OpenAPISpec   []byte
	AuthService   *auth.Service
	TeamRepo      team.Repository
	TierRepo      tier.Repository
	BlueprintRepo blueprint.Repository
	UserRepo      auth.UserRepository
}

// NewRouter creates and configures a Chi router with all middleware and routes.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(chimiddleware.Logger)

	// Public routes (no auth)
	healthHandler := handler.NewHealthHandler(deps.K8sChecker, deps.DBPinger, deps.Version)
	r.Get("/health", healthHandler.ServeHTTP)

	if len(deps.OpenAPISpec) > 0 {
		openapiHandler := handler.NewOpenAPIHandler(deps.OpenAPISpec)
		r.Get("/openapi.json", openapiHandler.ServeHTTP)
	}

	// Authenticated routes
	if deps.AuthService != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(deps.AuthService))

			// Superuser-only routes
			if deps.TeamRepo != nil {
				teamHandler := handler.NewTeamHandler(deps.TeamRepo)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireSuperuser())
					r.Post("/teams", teamHandler.Create)
					r.Get("/teams", teamHandler.List)
					r.Delete("/teams/{id}", teamHandler.Delete)

					if deps.UserRepo != nil {
						userHandler := handler.NewUserHandler(deps.AuthService, deps.UserRepo, deps.TeamRepo)
						r.Post("/users", userHandler.Create)
						r.Get("/users", userHandler.List)
						r.Delete("/users/{id}", userHandler.Delete)
					}
				})
			}

			// Business routes (platform + product)
			if deps.Repo != nil && deps.K8sManager != nil {
				dbHandler := handler.NewDatabaseHandler(deps.Repo, deps.K8sManager, deps.TeamRepo, deps.TierRepo, deps.Namespace)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireRole("platform", "product"))
					r.Post("/databases", dbHandler.Create)
					r.Get("/databases", dbHandler.List)
					r.Get("/databases/{id}", dbHandler.GetByID)
					r.Patch("/databases/{id}", dbHandler.Update)
					r.Delete("/databases/{id}", dbHandler.Delete)
				})
			}

			// Tier routes
			if deps.TierRepo != nil {
				tierHandler := handler.NewTierHandler(deps.TierRepo, deps.BlueprintRepo)

				// Read-only tier routes (platform + product)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireRole("platform", "product"))
					r.Get("/tiers", tierHandler.List)
					r.Get("/tiers/{id}", tierHandler.GetByID)
				})

				// Tier management routes (platform only)
				r.Group(func(r chi.Router) {
					r.Use(middleware.RequireRole("platform"))
					r.Post("/tiers", tierHandler.Create)
					r.Patch("/tiers/{id}", tierHandler.Update)
					r.Delete("/tiers/{id}", tierHandler.Delete)
				})
			}
		})
	} else {
		// Fallback: no auth service — register database routes without auth (graceful degradation)
		if deps.Repo != nil && deps.K8sManager != nil {
			dbHandler := handler.NewDatabaseHandler(deps.Repo, deps.K8sManager, deps.TeamRepo, deps.TierRepo, deps.Namespace)
			r.Route("/databases", func(r chi.Router) {
				r.Post("/", dbHandler.Create)
				r.Get("/", dbHandler.List)
				r.Get("/{id}", dbHandler.GetByID)
				r.Patch("/{id}", dbHandler.Update)
				r.Delete("/{id}", dbHandler.Delete)
			})
		}
	}

	return r
}
