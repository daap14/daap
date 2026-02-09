package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	clusterGVR = schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "clusters",
	}
	poolerGVR = schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "poolers",
	}
	secretGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}
)

// ClusterStatus represents the status of a CNPG Cluster resource.
type ClusterStatus struct {
	Phase string
	Ready bool
}

// ResourceManager manages CNPG Cluster and Pooler resources in Kubernetes.
type ResourceManager interface {
	ApplyCluster(ctx context.Context, cluster *unstructured.Unstructured) error
	ApplyPooler(ctx context.Context, pooler *unstructured.Unstructured) error
	DeleteCluster(ctx context.Context, namespace, name string) error
	DeletePooler(ctx context.Context, namespace, name string) error
	GetClusterStatus(ctx context.Context, namespace, name string) (ClusterStatus, error)
	GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

// Manager implements ResourceManager using the Kubernetes dynamic client.
type Manager struct {
	dynamic dynamic.Interface
}

// NewManager creates a ResourceManager from the existing Client.
func (c *Client) NewManager() *Manager {
	return &Manager{dynamic: c.dynamic}
}

// ApplyCluster creates or updates a CNPG Cluster resource.
func (m *Manager) ApplyCluster(ctx context.Context, cluster *unstructured.Unstructured) error {
	return m.apply(ctx, clusterGVR, cluster)
}

// ApplyPooler creates or updates a CNPG Pooler resource.
func (m *Manager) ApplyPooler(ctx context.Context, pooler *unstructured.Unstructured) error {
	return m.apply(ctx, poolerGVR, pooler)
}

// DeleteCluster deletes a CNPG Cluster resource. It does not error if the resource is not found.
func (m *Manager) DeleteCluster(ctx context.Context, namespace, name string) error {
	return m.delete(ctx, clusterGVR, namespace, name)
}

// DeletePooler deletes a CNPG Pooler resource. It does not error if the resource is not found.
func (m *Manager) DeletePooler(ctx context.Context, namespace, name string) error {
	return m.delete(ctx, poolerGVR, namespace, name)
}

// GetClusterStatus reads the status of a CNPG Cluster resource.
func (m *Manager) GetClusterStatus(ctx context.Context, namespace, name string) (ClusterStatus, error) {
	obj, err := m.dynamic.Resource(clusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ClusterStatus{}, fmt.Errorf("getting cluster %s/%s: %w", namespace, name, err)
	}

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")

	return ClusterStatus{
		Phase: phase,
		Ready: phase == "Cluster in healthy state",
	}, nil
}

// GetSecret reads a Kubernetes Secret and returns its data.
func (m *Manager) GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	obj, err := m.dynamic.Resource(secretGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting secret %s/%s: %w", namespace, name, err)
	}

	var secret corev1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &secret); err != nil {
		return nil, fmt.Errorf("converting secret %s/%s: %w", namespace, name, err)
	}

	return secret.Data, nil
}

// apply creates a resource; if it already exists, it updates it.
func (m *Manager) apply(ctx context.Context, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	namespace := obj.GetNamespace()
	name := obj.GetName()

	resource := m.dynamic.Resource(gvr).Namespace(namespace)

	_, err := resource.Create(ctx, obj, metav1.CreateOptions{})
	if err == nil {
		return nil
	}

	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}

	existing, err := resource.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting existing %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}

	return nil
}

// delete removes a resource; it does not error if the resource is not found.
func (m *Manager) delete(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) error {
	err := m.dynamic.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting %s %s/%s: %w", gvr.Resource, namespace, name, err)
	}
	return nil
}
