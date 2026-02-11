package validation_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/daap14/daap/internal/api/validation"
)

// --- ValidateCreateTierRequest ---

func validCreateTierRequest() validation.CreateTierRequest {
	return validation.CreateTierRequest{
		Name:                "standard",
		Description:         "Standard tier",
		BlueprintName:       "cnpg-standard",
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
	}
}

func TestCreateTier_Valid(t *testing.T) {
	t.Parallel()
	errs := validation.ValidateCreateTierRequest(validCreateTierRequest())
	assert.Empty(t, errs)
}

func TestCreateTier_NameRequired(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.Name = ""
	errs := validation.ValidateCreateTierRequest(req)
	assertFieldError(t, errs, "name", "required")
}

func TestCreateTier_NameRegex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"valid lowercase", "standard", true},
		{"valid with hyphen", "my-tier", true},
		{"valid with digits", "tier1", true},
		{"too short", "ab", false},
		{"single char", "a", false},
		{"uppercase", "Standard", false},
		{"underscore", "my_tier", false},
		{"starts with digit", "1tier", false},
		{"consecutive hyphens", "my--tier", false},
		{"63 chars", "a" + strings.Repeat("b", 62), true},
		{"64 chars", "a" + strings.Repeat("b", 63), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.Name = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "name")
			} else {
				assertHasFieldError(t, errs, "name")
			}
		})
	}
}

func TestCreateTier_BlueprintNameRequired(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.BlueprintName = ""
	errs := validation.ValidateCreateTierRequest(req)
	assertFieldError(t, errs, "blueprintName", "required")
}

func TestCreateTier_BlueprintNameWhitespace(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.BlueprintName = "   "
	errs := validation.ValidateCreateTierRequest(req)
	assertHasFieldError(t, errs, "blueprintName")
}

func TestCreateTier_DescriptionMaxLength(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.Description = strings.Repeat("a", 1001)
	errs := validation.ValidateCreateTierRequest(req)
	assertHasFieldError(t, errs, "description")
}

func TestCreateTier_DestructionStrategyEnum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"freeze", "freeze", true},
		{"archive", "archive", true},
		{"hard_delete", "hard_delete", true},
		{"invalid", "delete", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.DestructionStrategy = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "destructionStrategy")
			} else {
				assertHasFieldError(t, errs, "destructionStrategy")
			}
		})
	}
}

// --- ValidateUpdateTierRequest ---

func TestUpdateTier_EmptyRequest(t *testing.T) {
	t.Parallel()
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{})
	assert.Empty(t, errs)
}

func TestUpdateTier_ValidPartialUpdate(t *testing.T) {
	t.Parallel()
	desc := "Updated description"
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Description: &desc,
	})
	assert.Empty(t, errs)
}

func TestUpdateTier_InvalidDestructionStrategy(t *testing.T) {
	t.Parallel()
	val := "drop"
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		DestructionStrategy: &val,
	})
	assertHasFieldError(t, errs, "destructionStrategy")
}

func TestUpdateTier_EmptyDestructionStrategy(t *testing.T) {
	t.Parallel()
	val := ""
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		DestructionStrategy: &val,
	})
	assertHasFieldError(t, errs, "destructionStrategy")
}

func TestUpdateTier_DescriptionTooLong(t *testing.T) {
	t.Parallel()
	val := strings.Repeat("a", 1001)
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Description: &val,
	})
	assertHasFieldError(t, errs, "description")
}

// --- Test helpers ---

func assertFieldError(t *testing.T, errs []validation.FieldError, field, contains string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			assert.Contains(t, e.Message, contains)
			return
		}
	}
	t.Errorf("expected field error on %q containing %q, got none", field, contains)
}

func assertHasFieldError(t *testing.T, errs []validation.FieldError, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return
		}
	}
	t.Errorf("expected field error on %q, got none", field)
}

func assertNoFieldError(t *testing.T, errs []validation.FieldError, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			t.Errorf("expected no field error on %q, got: %s", field, e.Message)
			return
		}
	}
}
