package validation

import (
	"fmt"
	"strings"
)

var validDestructionStrategies = map[string]bool{"freeze": true, "archive": true, "hard_delete": true}

// CreateTierRequest mirrors the fields needed for create tier validation.
type CreateTierRequest struct {
	Name                string
	Description         string
	BlueprintName       string
	DestructionStrategy string
	BackupEnabled       bool
}

// ValidateCreateTierRequest validates the fields of a create tier request.
func ValidateCreateTierRequest(req CreateTierRequest) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(req.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "name is required"})
	} else if !NameRegex.MatchString(name) {
		errs = append(errs, FieldError{Field: "name", Message: "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"})
	} else if strings.Contains(name, "--") {
		errs = append(errs, FieldError{Field: "name", Message: "name must not contain consecutive hyphens"})
	}

	if len(req.Description) > 1000 {
		errs = append(errs, FieldError{Field: "description", Message: "description must be at most 1000 characters"})
	}

	if req.DestructionStrategy == "" {
		errs = append(errs, FieldError{Field: "destructionStrategy", Message: "destructionStrategy is required"})
	} else if !validDestructionStrategies[req.DestructionStrategy] {
		errs = append(errs, FieldError{Field: "destructionStrategy", Message: fmt.Sprintf("destructionStrategy must be one of: %s", joinKeys(validDestructionStrategies))})
	}

	return errs
}

// UpdateTierRequest mirrors the fields needed for update tier validation.
// Nil fields are not validated.
type UpdateTierRequest struct {
	Description         *string
	DestructionStrategy *string
	BackupEnabled       *bool
}

// ValidateUpdateTierRequest validates only non-nil fields on an update request.
func ValidateUpdateTierRequest(req UpdateTierRequest) []FieldError {
	var errs []FieldError

	if req.Description != nil && len(*req.Description) > 1000 {
		errs = append(errs, FieldError{Field: "description", Message: "description must be at most 1000 characters"})
	}

	if req.DestructionStrategy != nil {
		if *req.DestructionStrategy == "" {
			errs = append(errs, FieldError{Field: "destructionStrategy", Message: "destructionStrategy must not be empty"})
		} else if !validDestructionStrategies[*req.DestructionStrategy] {
			errs = append(errs, FieldError{Field: "destructionStrategy", Message: fmt.Sprintf("destructionStrategy must be one of: %s", joinKeys(validDestructionStrategies))})
		}
	}

	return errs
}

// joinKeys returns a sorted, comma-separated string of map keys.
func joinKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, fmt.Sprintf("%q", k))
	}
	// Sort for deterministic output
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return strings.Join(keys, ", ")
}
