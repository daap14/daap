package validation

import (
	"strings"

	"github.com/google/uuid"
)

// CreateUserRequest mirrors the fields needed for create user validation.
type CreateUserRequest struct {
	Name   string
	TeamID string
}

// ValidateCreateUserRequest validates the fields of a create user request.
func ValidateCreateUserRequest(req CreateUserRequest) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(req.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "name is required"})
	} else if len(name) > 255 {
		errs = append(errs, FieldError{Field: "name", Message: "name must be at most 255 characters"})
	}

	if req.TeamID == "" {
		errs = append(errs, FieldError{Field: "teamId", Message: "teamId is required"})
	} else if _, err := uuid.Parse(req.TeamID); err != nil {
		errs = append(errs, FieldError{Field: "teamId", Message: "teamId must be a valid UUID"})
	}

	return errs
}
