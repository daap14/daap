package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/k8s/template"
)

func TestBuildCluster_AllParamsSet(t *testing.T) {
	// Arrange
	params := template.ClusterParams{
		Name:      "mydb",
		Namespace: "production",
		PGVersion: "15",
	}

	// Act
	cluster := template.BuildCluster(params)

	// Assert — GVK
	assert.Equal(t, "postgresql.cnpg.io/v1", cluster.GetAPIVersion())
	assert.Equal(t, "Cluster", cluster.GetKind())

	// Assert — metadata
	assert.Equal(t, "daap-mydb", cluster.GetName())
	assert.Equal(t, "production", cluster.GetNamespace())

	labels := cluster.GetLabels()
	assert.Equal(t, "daap", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "mydb", labels["daap.io/database"])

	// Assert — spec uses platform defaults for instances and storage
	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(1), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:15", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1Gi", storage["size"])
}

func TestBuildCluster_Defaults(t *testing.T) {
	// Arrange — only required fields
	params := template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	}

	// Act
	cluster := template.BuildCluster(params)

	// Assert — defaults applied
	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(1), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:16", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1Gi", storage["size"])
}

func TestBuildCluster_CustomPGVersion(t *testing.T) {
	params := template.ClusterParams{
		Name:      "analytics",
		Namespace: "data",
		PGVersion: "14",
	}

	cluster := template.BuildCluster(params)

	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(1), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:14", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1Gi", storage["size"])
}

func TestBuildCluster_NamingConvention(t *testing.T) {
	tests := []struct {
		inputName    string
		expectedName string
	}{
		{"my-db", "daap-my-db"},
		{"postgres", "daap-postgres"},
		{"a", "daap-a"},
	}

	for _, tt := range tests {
		t.Run(tt.inputName, func(t *testing.T) {
			params := template.ClusterParams{
				Name:      tt.inputName,
				Namespace: "default",
			}

			cluster := template.BuildCluster(params)
			assert.Equal(t, tt.expectedName, cluster.GetName())
		})
	}
}
