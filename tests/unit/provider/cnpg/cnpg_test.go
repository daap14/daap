package cnpg_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	cnpgprovider "github.com/daap14/daap/internal/provider/cnpg"
	"github.com/daap14/daap/internal/provider"
)

// newFakeClient creates a fake dynamic client with CNPG types registered.
func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	// Register CNPG types
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Pooler"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "PoolerList"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ScheduledBackup"},
		{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ScheduledBackupList"},
	} {
		if gvk.Kind == "ClusterList" || gvk.Kind == "PoolerList" || gvk.Kind == "ScheduledBackupList" {
			scheme.AddKnownTypeWithName(gvk, &unstructured.UnstructuredList{})
		} else {
			scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		}
	}
	// Register core types
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "", Version: "v1", Kind: "ConfigMapList"},
	} {
		if gvk.Kind == "ConfigMapList" {
			scheme.AddKnownTypeWithName(gvk, &unstructured.UnstructuredList{})
		} else {
			scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		}
	}
	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

func sampleDB() provider.ProviderDatabase {
	return provider.ProviderDatabase{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Name:        "orders-db",
		Namespace:   "daap-system",
		ClusterName: "daap-orders-db",
		PoolerName:  "daap-orders-db-pooler",
		OwnerTeam:   "checkout",
		OwnerTeamID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Tier:        "production",
		TierID:      uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Blueprint:   "cnpg-prod-ha",
		Provider:    "cnpg",
	}
}

const singleDocManifest = `---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: daap-{{ .Name }}
  namespace: {{ .Namespace }}
spec:
  instances: 1
  imageName: ghcr.io/cloudnative-pg/postgresql:16
`

const multiDocManifest = `---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: daap-{{ .Name }}
  namespace: {{ .Namespace }}
spec:
  instances: 3
---
apiVersion: postgresql.cnpg.io/v1
kind: Pooler
metadata:
  name: daap-{{ .Name }}-pooler
  namespace: {{ .Namespace }}
spec:
  cluster:
    name: daap-{{ .Name }}
  type: rw
`

// --- Apply Tests ---

func TestApply_SingleDocument(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	err := p.Apply(context.Background(), db, singleDocManifest)
	require.NoError(t, err)

	// Verify the cluster was created with correct name and labels
	gvr := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	obj, err := client.Resource(gvr).Namespace("daap-system").Get(
		context.Background(), "daap-orders-db", metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "daap-orders-db", obj.GetName())
	assert.Equal(t, "orders-db", obj.GetLabels()["daap.io/database"])
	assert.Equal(t, "daap", obj.GetLabels()["app.kubernetes.io/managed-by"])
}

func TestApply_MultiDocument(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	err := p.Apply(context.Background(), db, multiDocManifest)
	require.NoError(t, err)

	// Verify cluster
	clusterGVR := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	cluster, err := client.Resource(clusterGVR).Namespace("daap-system").Get(
		context.Background(), "daap-orders-db", metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "daap-orders-db", cluster.GetName())

	// Verify pooler
	poolerGVR := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "poolers"}
	pooler, err := client.Resource(poolerGVR).Namespace("daap-system").Get(
		context.Background(), "daap-orders-db-pooler", metav1.GetOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "daap-orders-db-pooler", pooler.GetName())
}

func TestApply_TemplateVariableSubstitution(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	manifests := `---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: daap-{{ .Name }}
  namespace: {{ .Namespace }}
  annotations:
    owner-team: "{{ .OwnerTeam }}"
    tier: "{{ .Tier }}"
    blueprint: "{{ .Blueprint }}"
    provider: "{{ .Provider }}"
spec:
  instances: 1
`

	err := p.Apply(context.Background(), db, manifests)
	require.NoError(t, err)

	gvr := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	obj, err := client.Resource(gvr).Namespace("daap-system").Get(
		context.Background(), "daap-orders-db", metav1.GetOptions{},
	)
	require.NoError(t, err)

	annotations := obj.GetAnnotations()
	assert.Equal(t, "checkout", annotations["owner-team"])
	assert.Equal(t, "production", annotations["tier"])
	assert.Equal(t, "cnpg-prod-ha", annotations["blueprint"])
	assert.Equal(t, "cnpg", annotations["provider"])
}

func TestApply_PreservesExistingLabels(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	manifests := `---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: daap-{{ .Name }}
  namespace: {{ .Namespace }}
  labels:
    custom-label: custom-value
spec:
  instances: 1
`

	err := p.Apply(context.Background(), db, manifests)
	require.NoError(t, err)

	gvr := schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}
	obj, err := client.Resource(gvr).Namespace("daap-system").Get(
		context.Background(), "daap-orders-db", metav1.GetOptions{},
	)
	require.NoError(t, err)

	labels := obj.GetLabels()
	assert.Equal(t, "custom-value", labels["custom-label"])
	assert.Equal(t, "orders-db", labels["daap.io/database"])
	assert.Equal(t, "daap", labels["app.kubernetes.io/managed-by"])
}

func TestApply_InvalidTemplate(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	manifests := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .InvalidField }}
  namespace: {{ .Namespace }}
`

	err := p.Apply(context.Background(), db, manifests)
	assert.Error(t, err)
}

func TestApply_EmptyManifests(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	err := p.Apply(context.Background(), db, "---\n")
	assert.Error(t, err)
}

// --- Delete Tests ---

func TestDelete_RemovesLabeledResources(t *testing.T) {
	t.Parallel()

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]any{
				"name":      "daap-orders-db",
				"namespace": "daap-system",
				"labels": map[string]any{
					"daap.io/database":             "orders-db",
					"app.kubernetes.io/managed-by": "daap",
				},
			},
		},
	}

	client := newFakeClient(cluster)
	p := cnpgprovider.New(client)
	db := sampleDB()

	err := p.Delete(context.Background(), db)
	require.NoError(t, err)
}

func TestDelete_NoResources(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	err := p.Delete(context.Background(), db)
	require.NoError(t, err)
}

// --- CheckHealth Tests ---

func TestCheckHealth_Healthy(t *testing.T) {
	t.Parallel()

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]any{
				"name":      "daap-orders-db",
				"namespace": "daap-system",
			},
			"status": map[string]any{
				"phase": "Cluster in healthy state",
			},
		},
	}

	client := newFakeClient(cluster)
	p := cnpgprovider.New(client)
	db := sampleDB()

	result, err := p.CheckHealth(context.Background(), db)
	require.NoError(t, err)

	assert.Equal(t, "ready", result.Status)
	require.NotNil(t, result.Host)
	assert.Equal(t, "daap-orders-db-pooler.daap-system.svc.cluster.local", *result.Host)
	require.NotNil(t, result.Port)
	assert.Equal(t, 5432, *result.Port)
	require.NotNil(t, result.SecretName)
	assert.Equal(t, "daap-orders-db-app", *result.SecretName)
}

func TestCheckHealth_Provisioning(t *testing.T) {
	t.Parallel()

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]any{
				"name":      "daap-orders-db",
				"namespace": "daap-system",
			},
			"status": map[string]any{
				"phase": "Setting up primary",
			},
		},
	}

	client := newFakeClient(cluster)
	p := cnpgprovider.New(client)
	db := sampleDB()

	result, err := p.CheckHealth(context.Background(), db)
	require.NoError(t, err)

	assert.Equal(t, "provisioning", result.Status)
	assert.Nil(t, result.Host)
	assert.Nil(t, result.Port)
	assert.Nil(t, result.SecretName)
}

func TestCheckHealth_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		phase string
	}{
		{"failed", "Failed"},
		{"error", "Error"},
		{"unhealthy", "Cluster in unhealthy state"},
		{"failed-create", "Failed to create primary"},
		{"failed-reconcile", "Failed to reconcile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "postgresql.cnpg.io/v1",
					"kind":       "Cluster",
					"metadata": map[string]any{
						"name":      "daap-orders-db",
						"namespace": "daap-system",
					},
					"status": map[string]any{
						"phase": tt.phase,
					},
				},
			}

			client := newFakeClient(cluster)
			p := cnpgprovider.New(client)
			db := sampleDB()

			result, err := p.CheckHealth(context.Background(), db)
			require.NoError(t, err)
			assert.Equal(t, "error", result.Status)
		})
	}
}

func TestCheckHealth_ClusterNotFound(t *testing.T) {
	t.Parallel()
	client := newFakeClient()
	p := cnpgprovider.New(client)
	db := sampleDB()

	_, err := p.CheckHealth(context.Background(), db)
	assert.Error(t, err)
}
