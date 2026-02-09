package template

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PoolerParams configures a CNPG Pooler (PgBouncer) resource.
type PoolerParams struct {
	Name        string
	Namespace   string
	ClusterName string // the CNPG Cluster name (daap-{dbname})
}

// BuildPooler creates an unstructured CNPG Pooler resource from the given parameters.
func BuildPooler(params PoolerParams) *unstructured.Unstructured {
	name := fmt.Sprintf("daap-%s-pooler", params.Name)

	pooler := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Pooler",
			"metadata": map[string]any{
				"name":      name,
				"namespace": params.Namespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "daap",
					"daap.io/database":             params.Name,
				},
			},
			"spec": map[string]any{
				"cluster": map[string]any{
					"name": params.ClusterName,
				},
				"type": "rw",
				"pgbouncer": map[string]any{
					"poolMode": "transaction",
				},
			},
		},
	}

	return pooler
}
