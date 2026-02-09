package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/k8s/template"
)

func TestBuildPooler_GVKAndMetadata(t *testing.T) {
	// Arrange
	params := template.PoolerParams{
		Name:        "mydb",
		Namespace:   "production",
		ClusterName: "daap-mydb",
	}

	// Act
	pooler := template.BuildPooler(params)

	// Assert — GVK
	assert.Equal(t, "postgresql.cnpg.io/v1", pooler.GetAPIVersion())
	assert.Equal(t, "Pooler", pooler.GetKind())

	// Assert — metadata
	assert.Equal(t, "daap-mydb-pooler", pooler.GetName())
	assert.Equal(t, "production", pooler.GetNamespace())

	labels := pooler.GetLabels()
	assert.Equal(t, "daap", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "mydb", labels["daap.io/database"])
}

func TestBuildPooler_Spec(t *testing.T) {
	// Arrange
	params := template.PoolerParams{
		Name:        "orders",
		Namespace:   "staging",
		ClusterName: "daap-orders",
	}

	// Act
	pooler := template.BuildPooler(params)

	// Assert — spec
	spec, ok := pooler.Object["spec"].(map[string]any)
	require.True(t, ok)

	// Cluster reference
	clusterRef, ok := spec["cluster"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "daap-orders", clusterRef["name"])

	// Type
	assert.Equal(t, "rw", spec["type"])

	// PgBouncer config
	pgbouncer, ok := spec["pgbouncer"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "transaction", pgbouncer["poolMode"])
}

func TestBuildPooler_NamingConvention(t *testing.T) {
	tests := []struct {
		inputName    string
		expectedName string
	}{
		{"my-db", "daap-my-db-pooler"},
		{"postgres", "daap-postgres-pooler"},
		{"a", "daap-a-pooler"},
	}

	for _, tt := range tests {
		t.Run(tt.inputName, func(t *testing.T) {
			params := template.PoolerParams{
				Name:        tt.inputName,
				Namespace:   "default",
				ClusterName: "daap-" + tt.inputName,
			}

			pooler := template.BuildPooler(params)
			assert.Equal(t, tt.expectedName, pooler.GetName())
		})
	}
}
