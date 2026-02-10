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
		Name:        "mydb",
		Namespace:   "production",
		Instances:   3,
		CPU:         "1",
		Memory:      "4Gi",
		StorageSize: "10Gi",
		PGVersion:   "15",
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

	// Assert — spec uses provided params
	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(3), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:15", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "10Gi", storage["size"])

	resources, ok := spec["resources"].(map[string]any)
	require.True(t, ok)
	requests, ok := resources["requests"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", requests["cpu"])
	assert.Equal(t, "4Gi", requests["memory"])
	limits, ok := resources["limits"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", limits["cpu"])
	assert.Equal(t, "4Gi", limits["memory"])
}

func TestBuildCluster_MinimalParams(t *testing.T) {
	// Arrange — standard tier-like params
	params := template.ClusterParams{
		Name:        "testdb",
		Namespace:   "default",
		Instances:   1,
		CPU:         "500m",
		Memory:      "512Mi",
		StorageSize: "1Gi",
		PGVersion:   "16",
	}

	// Act
	cluster := template.BuildCluster(params)

	// Assert
	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(1), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:16", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1Gi", storage["size"])
	_, hasStorageClass := storage["storageClass"]
	assert.False(t, hasStorageClass, "empty StorageClass should not produce a storageClass key")
}

func TestBuildCluster_WithStorageClass(t *testing.T) {
	params := template.ClusterParams{
		Name:         "analytics",
		Namespace:    "data",
		Instances:    2,
		CPU:          "2",
		Memory:       "8Gi",
		StorageSize:  "100Gi",
		StorageClass: "fast-ssd",
		PGVersion:    "17",
	}

	cluster := template.BuildCluster(params)

	spec, ok := cluster.Object["spec"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, int64(2), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:17", spec["imageName"])

	storage, ok := spec["storage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "100Gi", storage["size"])
	assert.Equal(t, "fast-ssd", storage["storageClass"])
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
				Name:        tt.inputName,
				Namespace:   "default",
				Instances:   1,
				CPU:         "500m",
				Memory:      "512Mi",
				StorageSize: "1Gi",
				PGVersion:   "16",
			}

			cluster := template.BuildCluster(params)
			assert.Equal(t, tt.expectedName, cluster.GetName())
		})
	}
}
