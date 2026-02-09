package api

import (
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/k8s"
)

// NewRouter creates and configures a Chi router with all middleware and routes.
func NewRouter(k8sChecker k8s.HealthChecker, version string) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery)
	r.Use(chimiddleware.Logger)

	healthHandler := handler.NewHealthHandler(k8sChecker, version)
	r.Get("/health", healthHandler.ServeHTTP)

	return r
}
