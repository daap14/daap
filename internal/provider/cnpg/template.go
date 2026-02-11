package cnpg

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/daap14/daap/internal/provider"
)

// templateContext is the data passed to Go templates in blueprint manifests.
type templateContext struct {
	ID          string
	Name        string
	Namespace   string
	ClusterName string
	PoolerName  string
	OwnerTeam   string
	OwnerTeamID string
	Tier        string
	TierID      string
	Blueprint   string
	Provider    string
}

// toTemplateContext builds a templateContext from a ProviderDatabase.
func toTemplateContext(db provider.ProviderDatabase) templateContext {
	return templateContext{
		ID:          db.ID.String(),
		Name:        db.Name,
		Namespace:   db.Namespace,
		ClusterName: db.ClusterName,
		PoolerName:  db.PoolerName,
		OwnerTeam:   db.OwnerTeam,
		OwnerTeamID: db.OwnerTeamID.String(),
		Tier:        db.Tier,
		TierID:      db.TierID.String(),
		Blueprint:   db.Blueprint,
		Provider:    db.Provider,
	}
}

// renderManifests parses and executes Go templates in the manifests string
// using the provided ProviderDatabase context. Returns the rendered YAML.
func renderManifests(manifests string, db provider.ProviderDatabase) (string, error) {
	tmpl, err := template.New("blueprint").Parse(manifests)
	if err != nil {
		return "", fmt.Errorf("parsing blueprint template: %w", err)
	}

	ctx := toTemplateContext(db)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing blueprint template: %w", err)
	}

	return buf.String(), nil
}

// splitYAMLDocuments splits a multi-document YAML string on "---" separators.
// Empty documents are discarded.
func splitYAMLDocuments(yaml string) []string {
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
