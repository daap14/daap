package cnpg

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	labelDatabase       = "daap.io/database"
	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "daap"
)

// injectLabels adds mandatory DAAP labels to an unstructured K8s object,
// preserving any existing labels from the blueprint.
func injectLabels(obj *unstructured.Unstructured, databaseName string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[labelDatabase] = databaseName
	labels[labelManagedBy] = labelManagedByValue
	obj.SetLabels(labels)
}
