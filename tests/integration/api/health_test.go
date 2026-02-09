package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/k8s"
)

// mockHealthChecker implements k8s.HealthChecker for integration tests.
type mockHealthChecker struct {
	status k8s.ConnectivityStatus
}

func (m *mockHealthChecker) CheckConnectivity(_ context.Context) k8s.ConnectivityStatus {
	return m.status
}

// mockDBPinger implements handler.DBPinger for integration tests.
type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) Ping(_ context.Context) error {
	return m.err
}

// startTestServer creates an HTTP server on a random port and returns its base URL.
func startTestServer(t *testing.T, checker k8s.HealthChecker, pinger handler.DBPinger, version string) (string, *http.Server) {
	t.Helper()

	router := api.NewRouter(api.RouterDeps{
		K8sChecker: checker,
		DBPinger:   pinger,
		Version:    version,
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	return baseURL, srv
}

func TestHealthEndpoint_Healthy(t *testing.T) {
	// Arrange
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Act
	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var env map[string]interface{}
	err = json.Unmarshal(body, &env)
	require.NoError(t, err)

	// Verify envelope structure
	assert.Contains(t, env, "data")
	assert.Contains(t, env, "error")
	assert.Contains(t, env, "meta")
	assert.Nil(t, env["error"])

	// Verify data
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "healthy", data["status"])
	assert.Equal(t, "0.1.0", data["version"])

	k8sStatus := data["kubernetes"].(map[string]interface{})
	assert.Equal(t, true, k8sStatus["connected"])
	assert.Equal(t, "v1.31.0", k8sStatus["version"])

	dbStatus := data["database"].(map[string]interface{})
	assert.Equal(t, true, dbStatus["connected"])

	// Verify meta
	meta := env["meta"].(map[string]interface{})
	requestID := meta["requestId"].(string)
	_, uuidErr := uuid.Parse(requestID)
	assert.NoError(t, uuidErr, "requestId should be a valid UUID")
	assert.NotEmpty(t, meta["timestamp"])

	// Verify X-Request-ID response header matches meta
	assert.Equal(t, requestID, resp.Header.Get("X-Request-ID"))
}

func TestHealthEndpoint_Degraded(t *testing.T) {
	// Arrange
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: false},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Act
	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var env map[string]interface{}
	err = json.Unmarshal(body, &env)
	require.NoError(t, err)

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "degraded", data["status"])

	k8sStatus := data["kubernetes"].(map[string]interface{})
	assert.Equal(t, false, k8sStatus["connected"])
	assert.Nil(t, k8sStatus["version"])
}

func TestHealthEndpoint_ForwardsRequestID(t *testing.T) {
	// Arrange
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")
	defer func() { _ = srv.Shutdown(context.Background()) }()

	customID := "my-trace-id-12345"
	req, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", customID)

	// Act
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, customID, resp.Header.Get("X-Request-ID"))

	var env map[string]interface{}
	err = json.Unmarshal(body, &env)
	require.NoError(t, err)

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, customID, meta["requestId"])
}

func TestHealthEndpoint_NotFoundOnOtherPaths(t *testing.T) {
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")
	defer func() { _ = srv.Shutdown(context.Background()) }()

	resp, err := http.Get(baseURL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHealthEndpoint_GracefulShutdown(t *testing.T) {
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")

	// Verify server is running
	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Shutdown gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	assert.NoError(t, err)

	// After shutdown, requests should fail
	_, err = http.Get(baseURL + "/health")
	assert.Error(t, err, "requests should fail after shutdown")
}

func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &mockDBPinger{err: nil}
	baseURL, srv := startTestServer(t, checker, pinger, "0.1.0")
	defer func() { _ = srv.Shutdown(context.Background()) }()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/health", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
