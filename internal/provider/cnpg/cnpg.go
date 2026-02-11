package cnpg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/daap14/daap/internal/provider"
)

// Known CNPG-related GVRs for label-based deletion scanning.
var knownGVRs = []schema.GroupVersionResource{
	{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"},
	{Group: "postgresql.cnpg.io", Version: "v1", Resource: "poolers"},
	{Group: "postgresql.cnpg.io", Version: "v1", Resource: "scheduledbackups"},
	{Group: "", Version: "v1", Resource: "configmaps"},
}

// CNPGProvider implements the Provider interface for CloudNativePG.
type CNPGProvider struct {
	client dynamic.Interface
}

// New creates a new CNPG provider with the given dynamic K8s client.
func New(client dynamic.Interface) *CNPGProvider {
	return &CNPGProvider{client: client}
}

// Apply renders the blueprint manifests with the database context,
// injects mandatory labels, and creates or updates each K8s resource.
func (p *CNPGProvider) Apply(ctx context.Context, db provider.ProviderDatabase, manifests string) error {
	rendered, err := renderManifests(manifests, db)
	if err != nil {
		return fmt.Errorf("rendering manifests for %s: %w", db.Name, err)
	}

	docs := splitYAMLDocuments(rendered)
	if len(docs) == 0 {
		return fmt.Errorf("blueprint manifests for %s produced no documents", db.Name)
	}

	for i, doc := range docs {
		obj, err := parseUnstructured(doc)
		if err != nil {
			return fmt.Errorf("parsing document %d for %s: %w", i, db.Name, err)
		}

		injectLabels(obj, db.Name)

		if err := p.apply(ctx, obj); err != nil {
			return fmt.Errorf("applying document %d (%s/%s) for %s: %w",
				i, obj.GetKind(), obj.GetName(), db.Name, err)
		}
	}

	return nil
}

// Delete removes all K8s resources labeled with daap.io/database={name}
// in the database's namespace, scanning known CNPG GVRs.
func (p *CNPGProvider) Delete(ctx context.Context, db provider.ProviderDatabase) error {
	labelSelector := fmt.Sprintf("%s=%s", labelDatabase, db.Name)

	for _, gvr := range knownGVRs {
		list, err := p.client.Resource(gvr).Namespace(db.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			slog.Warn("cnpg provider: failed to list resources for deletion",
				"gvr", gvr.Resource, "database", db.Name, "error", err)
			continue
		}

		for _, item := range list.Items {
			err := p.client.Resource(gvr).Namespace(db.Namespace).Delete(
				ctx, item.GetName(), metav1.DeleteOptions{},
			)
			if err != nil && !k8serrors.IsNotFound(err) {
				slog.Warn("cnpg provider: failed to delete resource",
					"gvr", gvr.Resource, "name", item.GetName(),
					"database", db.Name, "error", err)
			}
		}
	}

	return nil
}

// CheckHealth reads the CNPG Cluster status and maps it to a HealthResult.
func (p *CNPGProvider) CheckHealth(ctx context.Context, db provider.ProviderDatabase) (provider.HealthResult, error) {
	clusterGVR := schema.GroupVersionResource{
		Group:    "postgresql.cnpg.io",
		Version:  "v1",
		Resource: "clusters",
	}

	obj, err := p.client.Resource(clusterGVR).Namespace(db.Namespace).Get(
		ctx, db.ClusterName, metav1.GetOptions{},
	)
	if err != nil {
		return provider.HealthResult{}, fmt.Errorf("getting cluster %s/%s: %w", db.Namespace, db.ClusterName, err)
	}

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")

	if phase == "Cluster in healthy state" {
		host := db.PoolerName + "." + db.Namespace + ".svc.cluster.local"
		port := 5432
		secretName := db.ClusterName + "-app"
		return provider.HealthResult{
			Status:     "ready",
			Host:       &host,
			Port:       &port,
			SecretName: &secretName,
		}, nil
	}

	if isFailedPhase(phase) {
		return provider.HealthResult{Status: "error"}, nil
	}

	return provider.HealthResult{Status: "provisioning"}, nil
}

// isFailedPhase determines whether a CNPG cluster phase indicates failure.
// Known healthy/transient phases return false; known failure phases return true.
// Unknown phases default to false (not failed) — the safe choice to avoid
// prematurely marking a database as errored.
func isFailedPhase(phase string) bool {
	switch phase {
	// Healthy or transient phases — not failed.
	case "Setting up primary", "Creating primary", "Cluster in healthy state":
		return false
	// Terminal failure phases.
	case "Failed", "Error",
		"Cluster in unhealthy state",
		"Failed to create primary",
		"Failed to reconcile":
		return true
	}
	// Unknown phase — default to not failed.
	return false
}

// parseUnstructured converts a single YAML document into an unstructured K8s object.
func parseUnstructured(yamlDoc string) (*unstructured.Unstructured, error) {
	jsonBytes, err := sigsyaml.YAMLToJSON([]byte(yamlDoc))
	if err != nil {
		return nil, fmt.Errorf("converting YAML to JSON: %w", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return nil, fmt.Errorf("unmarshalling JSON: %w", err)
	}

	return &unstructured.Unstructured{Object: obj}, nil
}

// apply creates a K8s resource; if it already exists, updates it.
// This reuses the same pattern as internal/k8s/manager.go:apply().
func (p *CNPGProvider) apply(ctx context.Context, obj *unstructured.Unstructured) error {
	gvr, err := gvrFromObject(obj)
	if err != nil {
		return err
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()
	resource := p.client.Resource(gvr).Namespace(namespace)

	_, err = resource.Create(ctx, obj, metav1.CreateOptions{})
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

// gvrFromObject derives a GroupVersionResource from an unstructured object's
// apiVersion and kind. This uses a known mapping for CNPG and core resources.
func gvrFromObject(obj *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	apiVersion := obj.GetAPIVersion()
	kind := obj.GetKind()

	key := apiVersion + "/" + kind
	gvr, ok := kindToGVR[key]
	if !ok {
		return schema.GroupVersionResource{}, fmt.Errorf("unknown resource kind %s (apiVersion: %s)", kind, apiVersion)
	}
	return gvr, nil
}

// kindToGVR maps apiVersion/kind combinations to their GVR.
var kindToGVR = map[string]schema.GroupVersionResource{
	"postgresql.cnpg.io/v1/Cluster":         {Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"},
	"postgresql.cnpg.io/v1/Pooler":          {Group: "postgresql.cnpg.io", Version: "v1", Resource: "poolers"},
	"postgresql.cnpg.io/v1/ScheduledBackup": {Group: "postgresql.cnpg.io", Version: "v1", Resource: "scheduledbackups"},
	"v1/ConfigMap":                          {Group: "", Version: "v1", Resource: "configmaps"},
	"v1/Secret":                             {Group: "", Version: "v1", Resource: "secrets"},
	"monitoring.coreos.com/v1/PodMonitor":   {Group: "monitoring.coreos.com", Version: "v1", Resource: "podmonitors"},
}
