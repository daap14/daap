package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/daap14/daap/internal/k8s/template"
)

// newTestManager creates a Manager backed by a fake dynamic client.
func newTestManager(objects ...runtime.Object) (*Manager, *dynamicfake.FakeDynamicClient) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Pooler"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "PoolerList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "", Version: "v1", Kind: "SecretList"},
		&unstructured.UnstructuredList{},
	)

	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	mgr := &Manager{dynamic: fakeClient}
	return mgr, fakeClient
}

// --- ApplyCluster Tests ---

func TestApplyCluster_Create(t *testing.T) {
	mgr, fakeClient := newTestManager()
	ctx := context.Background()

	cluster := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})

	err := mgr.ApplyCluster(ctx, cluster)
	require.NoError(t, err)

	// Verify the cluster was created
	obj, err := fakeClient.Resource(clusterGVR).Namespace("default").Get(ctx, "daap-testdb", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "daap-testdb", obj.GetName())
	assert.Equal(t, "Cluster", obj.GetKind())
}

func TestApplyCluster_UpdateExisting(t *testing.T) {
	ctx := context.Background()

	existing := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})

	mgr, fakeClient := newTestManager(existing)

	updated := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
		PGVersion: "15",
	})

	err := mgr.ApplyCluster(ctx, updated)
	require.NoError(t, err)

	// Verify the cluster was updated
	obj, err := fakeClient.Resource(clusterGVR).Namespace("default").Get(ctx, "daap-testdb", metav1.GetOptions{})
	require.NoError(t, err)

	spec, ok := obj.Object["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(1), spec["instances"])
	assert.Equal(t, "ghcr.io/cloudnative-pg/postgresql:15", spec["imageName"])
}

func TestApplyCluster_Error(t *testing.T) {
	mgr, fakeClient := newTestManager()
	ctx := context.Background()

	fakeClient.PrependReactor("create", "clusters", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})

	cluster := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})

	err := mgr.ApplyCluster(ctx, cluster)
	assert.Error(t, err)
}

// --- ApplyPooler Tests ---

func TestApplyPooler_Create(t *testing.T) {
	mgr, fakeClient := newTestManager()
	ctx := context.Background()

	pooler := template.BuildPooler(template.PoolerParams{
		Name:        "testdb",
		Namespace:   "default",
		ClusterName: "daap-testdb",
	})

	err := mgr.ApplyPooler(ctx, pooler)
	require.NoError(t, err)

	obj, err := fakeClient.Resource(poolerGVR).Namespace("default").Get(ctx, "daap-testdb-pooler", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "daap-testdb-pooler", obj.GetName())
	assert.Equal(t, "Pooler", obj.GetKind())
}

// --- DeleteCluster Tests ---

func TestDeleteCluster_Success(t *testing.T) {
	ctx := context.Background()

	existing := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})

	mgr, _ := newTestManager(existing)

	err := mgr.DeleteCluster(ctx, "default", "daap-testdb")
	require.NoError(t, err)
}

func TestDeleteCluster_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	ctx := context.Background()

	// Deleting a non-existent cluster should not error
	err := mgr.DeleteCluster(ctx, "default", "nonexistent")
	assert.NoError(t, err)
}

func TestDeleteCluster_Error(t *testing.T) {
	mgr, fakeClient := newTestManager()
	ctx := context.Background()

	fakeClient.PrependReactor("delete", "clusters", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})

	err := mgr.DeleteCluster(ctx, "default", "testdb")
	assert.Error(t, err)
}

// --- DeletePooler Tests ---

func TestDeletePooler_Success(t *testing.T) {
	ctx := context.Background()

	existing := template.BuildPooler(template.PoolerParams{
		Name:        "testdb",
		Namespace:   "default",
		ClusterName: "daap-testdb",
	})

	mgr, _ := newTestManager(existing)

	err := mgr.DeletePooler(ctx, "default", "daap-testdb-pooler")
	require.NoError(t, err)
}

func TestDeletePooler_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	ctx := context.Background()

	err := mgr.DeletePooler(ctx, "default", "nonexistent-pooler")
	assert.NoError(t, err)
}

func TestDeletePooler_Error(t *testing.T) {
	mgr, fakeClient := newTestManager()
	ctx := context.Background()

	fakeClient.PrependReactor("delete", "poolers", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})

	err := mgr.DeletePooler(ctx, "default", "testdb-pooler")
	assert.Error(t, err)
}

// --- GetClusterStatus Tests ---

func TestGetClusterStatus_Ready(t *testing.T) {
	ctx := context.Background()

	cluster := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})
	cluster.Object["status"] = map[string]any{
		"phase": "Cluster in healthy state",
	}

	mgr, _ := newTestManager(cluster)

	status, err := mgr.GetClusterStatus(ctx, "default", "daap-testdb")
	require.NoError(t, err)
	assert.True(t, status.Ready)
	assert.Equal(t, "Cluster in healthy state", status.Phase)
}

func TestGetClusterStatus_NotReady(t *testing.T) {
	ctx := context.Background()

	cluster := template.BuildCluster(template.ClusterParams{
		Name:      "testdb",
		Namespace: "default",
	})
	cluster.Object["status"] = map[string]any{
		"phase": "Setting up primary",
	}

	mgr, _ := newTestManager(cluster)

	status, err := mgr.GetClusterStatus(ctx, "default", "daap-testdb")
	require.NoError(t, err)
	assert.False(t, status.Ready)
	assert.Equal(t, "Setting up primary", status.Phase)
}

func TestGetClusterStatus_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	ctx := context.Background()

	_, err := mgr.GetClusterStatus(ctx, "default", "nonexistent")
	assert.Error(t, err)
}

// --- GetSecret Tests ---

func TestGetSecret_Success(t *testing.T) {
	ctx := context.Background()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daap-testdb-app",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("app"),
			"password": []byte("secret123"),
			"host":     []byte("daap-testdb-rw.default.svc"),
		},
	}

	unstrObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	require.NoError(t, err)
	unstr := &unstructured.Unstructured{Object: unstrObj}
	unstr.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})

	mgr, _ := newTestManager(unstr)

	data, err := mgr.GetSecret(ctx, "default", "daap-testdb-app")
	require.NoError(t, err)
	assert.Equal(t, []byte("app"), data["username"])
	assert.Equal(t, []byte("secret123"), data["password"])
	assert.Equal(t, []byte("daap-testdb-rw.default.svc"), data["host"])
}

func TestGetSecret_NotFound(t *testing.T) {
	mgr, _ := newTestManager()
	ctx := context.Background()

	_, err := mgr.GetSecret(ctx, "default", "nonexistent-secret")
	assert.Error(t, err)
}
