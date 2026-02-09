package template

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Platform defaults for CNPG cluster resources.
const (
	defaultInstances   = 1
	defaultStorageSize = "1Gi"
	defaultPGVersion   = "16"
)

// ClusterParams configures a CNPG Cluster resource.
type ClusterParams struct {
	Name      string
	Namespace string
	PGVersion string // e.g., "16"
}

// BuildCluster creates an unstructured CNPG Cluster resource from the given parameters.
// Instances and StorageSize are platform defaults and not configurable by consumers.
func BuildCluster(params ClusterParams) *unstructured.Unstructured {
	if params.PGVersion == "" {
		params.PGVersion = defaultPGVersion
	}

	name := fmt.Sprintf("daap-%s", params.Name)
	imageName := fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s", params.PGVersion)

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
				"instances":  int64(defaultInstances),
				"imageName":  imageName,
				"postgresql": map[string]any{},
				"storage": map[string]any{
					"size": defaultStorageSize,
				},
			},
		},
	}

	return cluster
}
