package validation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/daap14/daap/internal/api/validation"
	"github.com/daap14/daap/internal/provider"
)

func registryWith(providers ...string) *provider.Registry {
	reg := provider.NewRegistry()
	for _, p := range providers {
		reg.Register(p, nil)
	}
	return reg
}

const validManifests = `apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: test-cluster
spec:
  instances: 1`

func TestValidateCreateBlueprintRequest_Valid(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "cnpg-standard",
		Provider:  "cnpg",
		Manifests: validManifests,
		Registry:  registryWith("cnpg"),
	})

	assert.Empty(t, errs)
}

func TestValidateCreateBlueprintRequest_MissingFields(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "",
		Provider:  "",
		Manifests: "",
		Registry:  registryWith("cnpg"),
	})

	assert.Len(t, errs, 3)
	fields := make(map[string]string)
	for _, e := range errs {
		fields[e.Field] = e.Message
	}
	assert.Contains(t, fields, "name")
	assert.Contains(t, fields, "provider")
	assert.Contains(t, fields, "manifests")
}

func TestValidateCreateBlueprintRequest_InvalidName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{"uppercase", "INVALID", "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"},
		{"too short", "ab", "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"},
		{"starts with number", "1abc", "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"},
		{"consecutive hyphens", "my--blueprint", "name must not contain consecutive hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
				Name:      tt.input,
				Provider:  "cnpg",
				Manifests: validManifests,
				Registry:  registryWith("cnpg"),
			})
			var nameErrors []string
			for _, e := range errs {
				if e.Field == "name" {
					nameErrors = append(nameErrors, e.Message)
				}
			}
			assert.Contains(t, nameErrors, tt.wantMsg)
		})
	}
}

func TestValidateCreateBlueprintRequest_UnknownProvider(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "unknown",
		Manifests: validManifests,
		Registry:  registryWith("cnpg"),
	})

	assert.Len(t, errs, 1)
	assert.Equal(t, "provider", errs[0].Field)
	assert.Equal(t, "provider must be a registered provider", errs[0].Message)
}

func TestValidateCreateBlueprintRequest_NilRegistry_SkipsProviderCheck(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "anything",
		Manifests: validManifests,
		Registry:  nil,
	})

	assert.Empty(t, errs)
}

func TestValidateCreateBlueprintRequest_InvalidGoTemplate(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "cnpg",
		Manifests: "{{ .Name",
		Registry:  registryWith("cnpg"),
	})

	var manifestErrors []string
	for _, e := range errs {
		if e.Field == "manifests" {
			manifestErrors = append(manifestErrors, e.Message)
		}
	}
	assert.Contains(t, manifestErrors, "manifests contains invalid Go template syntax")
}

func TestValidateCreateBlueprintRequest_MissingAPIVersion(t *testing.T) {
	t.Parallel()

	manifests := `kind: Cluster
metadata:
  name: test`

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "cnpg",
		Manifests: manifests,
		Registry:  registryWith("cnpg"),
	})

	var manifestErrors []string
	for _, e := range errs {
		if e.Field == "manifests" {
			manifestErrors = append(manifestErrors, e.Message)
		}
	}
	assert.Contains(t, manifestErrors, "document 0 is missing apiVersion")
}

func TestValidateCreateBlueprintRequest_MissingKind(t *testing.T) {
	t.Parallel()

	manifests := `apiVersion: v1
metadata:
  name: test`

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "cnpg",
		Manifests: manifests,
		Registry:  registryWith("cnpg"),
	})

	var manifestErrors []string
	for _, e := range errs {
		if e.Field == "manifests" {
			manifestErrors = append(manifestErrors, e.Message)
		}
	}
	assert.Contains(t, manifestErrors, "document 0 is missing kind")
}

func TestValidateCreateBlueprintRequest_MissingMetadata(t *testing.T) {
	t.Parallel()

	manifests := `apiVersion: v1
kind: Cluster`

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "test-bp",
		Provider:  "cnpg",
		Manifests: manifests,
		Registry:  registryWith("cnpg"),
	})

	var manifestErrors []string
	for _, e := range errs {
		if e.Field == "manifests" {
			manifestErrors = append(manifestErrors, e.Message)
		}
	}
	assert.Contains(t, manifestErrors, "document 0 is missing metadata")
}

func TestValidateCreateBlueprintRequest_MultiDoc(t *testing.T) {
	t.Parallel()

	manifests := `apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: cluster
spec:
  instances: 1
---
apiVersion: postgresql.cnpg.io/v1
kind: Pooler
metadata:
  name: pooler
spec:
  type: rw`

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "multi-doc-bp",
		Provider:  "cnpg",
		Manifests: manifests,
		Registry:  registryWith("cnpg"),
	})

	assert.Empty(t, errs)
}

func TestValidateCreateBlueprintRequest_TemplatePlaceholders_SkipStructural(t *testing.T) {
	t.Parallel()

	manifests := `apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: {{ .Name }}
spec:
  instances: {{ .Instances }}`

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "template-bp",
		Provider:  "cnpg",
		Manifests: manifests,
		Registry:  registryWith("cnpg"),
	})

	// Template placeholders cause YAML parse to fail, so structural checks are skipped
	assert.Empty(t, errs)
}

func TestValidateCreateBlueprintRequest_WhitespaceTrimmingOnName(t *testing.T) {
	t.Parallel()

	errs := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      "  test-bp  ",
		Provider:  "cnpg",
		Manifests: validManifests,
		Registry:  registryWith("cnpg"),
	})

	assert.Empty(t, errs)
}
