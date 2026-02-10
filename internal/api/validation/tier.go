package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// K8s resource quantity patterns.
var (
	cpuRegex     = regexp.MustCompile(`^([0-9]+\.?[0-9]*)(m?)$`)
	memoryRegex  = regexp.MustCompile(`^[0-9]+(Ki|Mi|Gi|Ti|Pi|Ei)?$`)
	storageRegex = regexp.MustCompile(`^[0-9]+(Ki|Mi|Gi|Ti|Pi|Ei)?$`)
)

var validPGVersions = map[string]bool{"15": true, "16": true, "17": true}
var validPoolModes = map[string]bool{"transaction": true, "session": true, "statement": true}
var validDestructionStrategies = map[string]bool{"freeze": true, "archive": true, "hard_delete": true}

// CreateTierRequest mirrors the fields needed for create tier validation.
type CreateTierRequest struct {
	Name                string
	Description         string
	Instances           int
	CPU                 string
	Memory              string
	StorageSize         string
	StorageClass        string
	PGVersion           string
	PoolMode            string
	MaxConnections      int
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

	if req.Instances < 1 || req.Instances > 10 {
		errs = append(errs, FieldError{Field: "instances", Message: "instances must be between 1 and 10"})
	}

	cpu := strings.TrimSpace(req.CPU)
	if cpu == "" {
		errs = append(errs, FieldError{Field: "cpu", Message: "cpu is required"})
	} else if !cpuRegex.MatchString(cpu) {
		errs = append(errs, FieldError{Field: "cpu", Message: "cpu must be a valid K8s resource quantity (e.g., \"500m\", \"1\", \"2.5\")"})
	}

	memory := strings.TrimSpace(req.Memory)
	if memory == "" {
		errs = append(errs, FieldError{Field: "memory", Message: "memory is required"})
	} else if !memoryRegex.MatchString(memory) {
		errs = append(errs, FieldError{Field: "memory", Message: "memory must be a valid K8s resource quantity (e.g., \"512Mi\", \"1Gi\")"})
	}

	storageSize := strings.TrimSpace(req.StorageSize)
	if storageSize == "" {
		errs = append(errs, FieldError{Field: "storageSize", Message: "storageSize is required"})
	} else if !storageRegex.MatchString(storageSize) {
		errs = append(errs, FieldError{Field: "storageSize", Message: "storageSize must be a valid K8s storage format (e.g., \"1Gi\", \"500Mi\")"})
	}

	if len(req.StorageClass) > 255 {
		errs = append(errs, FieldError{Field: "storageClass", Message: "storageClass must be at most 255 characters"})
	}

	if req.PGVersion == "" {
		errs = append(errs, FieldError{Field: "pgVersion", Message: "pgVersion is required"})
	} else if !validPGVersions[req.PGVersion] {
		errs = append(errs, FieldError{Field: "pgVersion", Message: fmt.Sprintf("pgVersion must be one of: %s", joinKeys(validPGVersions))})
	}

	if req.PoolMode == "" {
		errs = append(errs, FieldError{Field: "poolMode", Message: "poolMode is required"})
	} else if !validPoolModes[req.PoolMode] {
		errs = append(errs, FieldError{Field: "poolMode", Message: fmt.Sprintf("poolMode must be one of: %s", joinKeys(validPoolModes))})
	}

	if req.MaxConnections < 10 || req.MaxConnections > 10000 {
		errs = append(errs, FieldError{Field: "maxConnections", Message: "maxConnections must be between 10 and 10000"})
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
	Instances           *int
	CPU                 *string
	Memory              *string
	StorageSize         *string
	StorageClass        *string
	PGVersion           *string
	PoolMode            *string
	MaxConnections      *int
	DestructionStrategy *string
	BackupEnabled       *bool
}

// ValidateUpdateTierRequest validates only non-nil fields on an update request.
func ValidateUpdateTierRequest(req UpdateTierRequest) []FieldError {
	var errs []FieldError

	if req.Description != nil && len(*req.Description) > 1000 {
		errs = append(errs, FieldError{Field: "description", Message: "description must be at most 1000 characters"})
	}

	if req.Instances != nil && (*req.Instances < 1 || *req.Instances > 10) {
		errs = append(errs, FieldError{Field: "instances", Message: "instances must be between 1 and 10"})
	}

	if req.CPU != nil {
		cpu := strings.TrimSpace(*req.CPU)
		if cpu == "" {
			errs = append(errs, FieldError{Field: "cpu", Message: "cpu must not be empty"})
		} else if !cpuRegex.MatchString(cpu) {
			errs = append(errs, FieldError{Field: "cpu", Message: "cpu must be a valid K8s resource quantity (e.g., \"500m\", \"1\", \"2.5\")"})
		}
	}

	if req.Memory != nil {
		memory := strings.TrimSpace(*req.Memory)
		if memory == "" {
			errs = append(errs, FieldError{Field: "memory", Message: "memory must not be empty"})
		} else if !memoryRegex.MatchString(memory) {
			errs = append(errs, FieldError{Field: "memory", Message: "memory must be a valid K8s resource quantity (e.g., \"512Mi\", \"1Gi\")"})
		}
	}

	if req.StorageSize != nil {
		storageSize := strings.TrimSpace(*req.StorageSize)
		if storageSize == "" {
			errs = append(errs, FieldError{Field: "storageSize", Message: "storageSize must not be empty"})
		} else if !storageRegex.MatchString(storageSize) {
			errs = append(errs, FieldError{Field: "storageSize", Message: "storageSize must be a valid K8s storage format (e.g., \"1Gi\", \"500Mi\")"})
		}
	}

	if req.StorageClass != nil && len(*req.StorageClass) > 255 {
		errs = append(errs, FieldError{Field: "storageClass", Message: "storageClass must be at most 255 characters"})
	}

	if req.PGVersion != nil {
		if *req.PGVersion == "" {
			errs = append(errs, FieldError{Field: "pgVersion", Message: "pgVersion must not be empty"})
		} else if !validPGVersions[*req.PGVersion] {
			errs = append(errs, FieldError{Field: "pgVersion", Message: fmt.Sprintf("pgVersion must be one of: %s", joinKeys(validPGVersions))})
		}
	}

	if req.PoolMode != nil {
		if *req.PoolMode == "" {
			errs = append(errs, FieldError{Field: "poolMode", Message: "poolMode must not be empty"})
		} else if !validPoolModes[*req.PoolMode] {
			errs = append(errs, FieldError{Field: "poolMode", Message: fmt.Sprintf("poolMode must be one of: %s", joinKeys(validPoolModes))})
		}
	}

	if req.MaxConnections != nil && (*req.MaxConnections < 10 || *req.MaxConnections > 10000) {
		errs = append(errs, FieldError{Field: "maxConnections", Message: "maxConnections must be between 10 and 10000"})
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
