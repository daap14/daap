package handler

import (
	"context"
	"net/http"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/k8s"
)

// DBPinger checks platform database connectivity.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler handles the GET /health endpoint.
type HealthHandler struct {
	k8sChecker k8s.HealthChecker
	dbPinger   DBPinger
	version    string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(checker k8s.HealthChecker, dbPinger DBPinger, version string) *HealthHandler {
	return &HealthHandler{
		k8sChecker: checker,
		dbPinger:   dbPinger,
		version:    version,
	}
}

type kubernetesStatus struct {
	Connected bool    `json:"connected"`
	Version   *string `json:"version"`
}

type databaseStatus struct {
	Connected bool `json:"connected"`
}

type healthData struct {
	Status     string           `json:"status"`
	Version    string           `json:"version"`
	Kubernetes kubernetesStatus `json:"kubernetes"`
	Database   databaseStatus   `json:"database"`
}

// ServeHTTP handles the health check request.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	connectivity := h.k8sChecker.CheckConnectivity(r.Context())

	status := "healthy"
	var k8sVersion *string

	if connectivity.Connected {
		k8sVersion = &connectivity.Version
	} else {
		status = "degraded"
	}

	dbConnected := true
	if h.dbPinger != nil {
		if err := h.dbPinger.Ping(r.Context()); err != nil {
			dbConnected = false
			status = "degraded"
		}
	} else {
		dbConnected = false
		status = "degraded"
	}

	data := healthData{
		Status:  status,
		Version: h.version,
		Kubernetes: kubernetesStatus{
			Connected: connectivity.Connected,
			Version:   k8sVersion,
		},
		Database: databaseStatus{
			Connected: dbConnected,
		},
	}

	response.Success(w, http.StatusOK, data, requestID)
}
