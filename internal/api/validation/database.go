package validation

import (
	"regexp"
	"strings"
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{1,61}[a-z0-9]$`)

// FieldError represents a validation error on a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// CreateDatabaseRequest mirrors the fields needed for create validation.
type CreateDatabaseRequest struct {
	Name      string
	OwnerTeam string
}

// ValidateCreateRequest validates the fields of a create database request.
// Returns a slice of field errors; empty slice means valid.
func ValidateCreateRequest(req CreateDatabaseRequest) []FieldError {
	var errs []FieldError

	if req.Name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "name is required"})
	} else if !nameRegex.MatchString(req.Name) {
		errs = append(errs, FieldError{Field: "name", Message: "name must be lowercase alphanumeric with hyphens, 3-63 characters, starting with a letter"})
	} else if strings.Contains(req.Name, "--") {
		errs = append(errs, FieldError{Field: "name", Message: "name must not contain consecutive hyphens"})
	}

	if req.OwnerTeam == "" {
		errs = append(errs, FieldError{Field: "ownerTeam", Message: "ownerTeam is required"})
	}

	return errs
}
