package template

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ClusterParams configures a CNPG Cluster resource.
// All fields must be explicitly provided — there are no silent defaults.
type ClusterParams struct {
	Name         string
	Namespace    string
	Instances    int    // CNPG cluster replica count
	CPU          string // K8s resource quantity (e.g., "500m", "2")
	Memory       string // K8s resource quantity (e.g., "512Mi", "4Gi")
	StorageSize  string // K8s storage size (e.g., "1Gi", "100Gi")
	StorageClass string // K8s StorageClass name; empty = cluster default
	PGVersion    string // PostgreSQL major version (e.g., "16")
}

// BuildCluster creates an unstructured CNPG Cluster resource from the given parameters.
// All infrastructure values come from the tier — no hardcoded defaults.
func BuildCluster(params ClusterParams) *unstructured.Unstructured {
	name := fmt.Sprintf("daap-%s", params.Name)
	imageName := fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s", params.PGVersion)

	storage := map[string]any{
		"size": params.StorageSize,
	}
	if params.StorageClass != "" {
		storage["storageClass"] = params.StorageClass
	}

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]any{
				"name":      name,
				"namespace": params.Namespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "daap",
					"daap.io/database":             params.Name,
				},
			},
			"spec": map[string]any{
				"instances":  int64(params.Instances),
				"imageName":  imageName,
				"postgresql": map[string]any{},
				"storage":    storage,
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    params.CPU,
						"memory": params.Memory,
					},
					"limits": map[string]any{
						"cpu":    params.CPU,
						"memory": params.Memory,
					},
				},
			},
		},
	}

	return cluster
}
