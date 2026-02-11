package validation

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/daap14/daap/internal/provider"
	sigsyaml "sigs.k8s.io/yaml"
)

// CreateBlueprintRequest mirrors the fields needed for create blueprint validation.
type CreateBlueprintRequest struct {
	Name      string
	Provider  string
	Manifests string
	Registry  *provider.Registry
}

// ValidateCreateBlueprintRequest validates the fields of a create blueprint request.
func ValidateCreateBlueprintRequest(req CreateBlueprintRequest) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(req.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "name is required"})
	} else if !NameRegex.MatchString(name) {
		errs = append(errs, FieldError{Field: "name", Message: "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"})
	} else if strings.Contains(name, "--") {
		errs = append(errs, FieldError{Field: "name", Message: "name must not contain consecutive hyphens"})
	}

	providerName := strings.TrimSpace(req.Provider)
	if providerName == "" {
		errs = append(errs, FieldError{Field: "provider", Message: "provider is required"})
	} else if req.Registry != nil && !req.Registry.Has(providerName) {
		errs = append(errs, FieldError{Field: "provider", Message: "provider must be a registered provider"})
	}

	manifests := strings.TrimSpace(req.Manifests)
	if manifests == "" {
		errs = append(errs, FieldError{Field: "manifests", Message: "manifests is required"})
	} else {
		errs = append(errs, validateManifests(manifests)...)
	}

	return errs
}

// validateManifests checks that the manifests string is valid multi-doc YAML,
// each document has apiVersion/kind/metadata.name, and Go templates parse.
func validateManifests(manifests string) []FieldError {
	var errs []FieldError

	// Check Go template parses
	if _, err := template.New("check").Parse(manifests); err != nil {
		errs = append(errs, FieldError{Field: "manifests", Message: "manifests contains invalid Go template syntax"})
		return errs
	}

	// Split on --- separators
	docs := splitYAMLDocs(manifests)
	if len(docs) == 0 {
		errs = append(errs, FieldError{Field: "manifests", Message: "manifests must contain at least one YAML document"})
		return errs
	}

	for i, doc := range docs {
		// Try to parse as YAML — templates like {{ .Name }} may cause YAML parse errors,
		// so we only validate structural fields that don't contain template placeholders.
		var obj map[string]any
		if err := sigsyaml.Unmarshal([]byte(doc), &obj); err != nil {
			// If YAML fails to parse, it may contain template placeholders — skip structural checks
			continue
		}

		if _, ok := obj["apiVersion"]; !ok {
			errs = append(errs, FieldError{
				Field:   "manifests",
				Message: fmt.Sprintf("document %d is missing apiVersion", i),
			})
		}
		if _, ok := obj["kind"]; !ok {
			errs = append(errs, FieldError{
				Field:   "manifests",
				Message: fmt.Sprintf("document %d is missing kind", i),
			})
		}
		if meta, ok := obj["metadata"].(map[string]any); ok {
			if _, ok := meta["name"]; !ok {
				errs = append(errs, FieldError{
					Field:   "manifests",
					Message: fmt.Sprintf("document %d is missing metadata.name", i),
				})
			}
		} else if _, ok := obj["metadata"]; !ok {
			errs = append(errs, FieldError{
				Field:   "manifests",
				Message: fmt.Sprintf("document %d is missing metadata", i),
			})
		}
	}

	return errs
}

// splitYAMLDocs splits a multi-document YAML string on "---" separators.
func splitYAMLDocs(yaml string) []string {
	parts := strings.Split(yaml, "\n---")
	var docs []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		docs = append(docs, trimmed)
	}
	return docs
}
