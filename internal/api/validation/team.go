package validation

import "strings"

// CreateTeamRequest mirrors the fields needed for create team validation.
type CreateTeamRequest struct {
	Name string
	Role string
}

// ValidateCreateTeamRequest validates the fields of a create team request.
func ValidateCreateTeamRequest(req CreateTeamRequest) []FieldError {
	var errs []FieldError

	name := strings.TrimSpace(req.Name)
	if name == "" {
		errs = append(errs, FieldError{Field: "name", Message: "name is required"})
	} else if len(name) > 255 {
		errs = append(errs, FieldError{Field: "name", Message: "name must be at most 255 characters"})
	}

	if req.Role == "" {
		errs = append(errs, FieldError{Field: "role", Message: "role is required"})
	} else if req.Role != "platform" && req.Role != "product" {
		errs = append(errs, FieldError{Field: "role", Message: "role must be \"platform\" or \"product\""})
	}

	return errs
}
