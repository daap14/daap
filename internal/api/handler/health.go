package handler

import (
	"net/http"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/k8s"
)

// HealthHandler handles the GET /health endpoint.
type HealthHandler struct {
	k8sChecker k8s.HealthChecker
	version    string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(checker k8s.HealthChecker, version string) *HealthHandler {
	return &HealthHandler{
		k8sChecker: checker,
		version:    version,
	}
}

type kubernetesStatus struct {
	Connected bool    `json:"connected"`
	Version   *string `json:"version"`
}

type healthData struct {
	Status     string           `json:"status"`
	Version    string           `json:"version"`
	Kubernetes kubernetesStatus `json:"kubernetes"`
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

	data := healthData{
		Status:  status,
		Version: h.version,
		Kubernetes: kubernetesStatus{
			Connected: connectivity.Connected,
			Version:   k8sVersion,
		},
	}

	response.Success(w, http.StatusOK, data, requestID)
}
