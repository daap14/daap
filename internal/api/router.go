package api

import (
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
)

// RouterDeps holds all dependencies needed by the router.
type RouterDeps struct {
	K8sChecker  k8s.HealthChecker
	DBPinger    handler.DBPinger
	Version     string
	Repo        database.Repository
	K8sManager  k8s.ResourceManager
	Namespace   string
	OpenAPISpec []byte
}

// NewRouter creates and configures a Chi router with all middleware and routes.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(chimiddleware.Logger)

	healthHandler := handler.NewHealthHandler(deps.K8sChecker, deps.DBPinger, deps.Version)
	r.Get("/health", healthHandler.ServeHTTP)

	if len(deps.OpenAPISpec) > 0 {
		openapiHandler := handler.NewOpenAPIHandler(deps.OpenAPISpec)
		r.Get("/openapi.json", openapiHandler.ServeHTTP)
	}

	if deps.Repo != nil && deps.K8sManager != nil {
		dbHandler := handler.NewDatabaseHandler(deps.Repo, deps.K8sManager, deps.Namespace)
		r.Route("/databases", func(r chi.Router) {
			r.Post("/", dbHandler.Create)
			r.Get("/", dbHandler.List)
			r.Get("/{id}", dbHandler.GetByID)
			r.Patch("/{id}", dbHandler.Update)
			r.Delete("/{id}", dbHandler.Delete)
		})
	}

	return r
}
