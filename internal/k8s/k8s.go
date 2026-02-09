package k8s

import "context"

// HealthChecker provides Kubernetes connectivity checking.
type HealthChecker interface {
	CheckConnectivity(ctx context.Context) ConnectivityStatus
}
