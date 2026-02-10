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
		Instances:           1,
		CPU:                 "500m",
		Memory:              "512Mi",
		StorageSize:         "1Gi",
		StorageClass:        "gp3",
		PGVersion:           "16",
		PoolMode:            "transaction",
		MaxConnections:      100,
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

func TestCreateTier_InstancesBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value int
		valid bool
	}{
		{"zero", 0, false},
		{"one", 1, true},
		{"five", 5, true},
		{"ten", 10, true},
		{"eleven", 11, false},
		{"negative", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.Instances = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "instances")
			} else {
				assertHasFieldError(t, errs, "instances")
			}
		})
	}
}

func TestCreateTier_CPUFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"millicores", "500m", true},
		{"whole", "1", true},
		{"decimal", "2.5", true},
		{"invalid unit", "500x", false},
		{"no number", "m", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.CPU = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "cpu")
			} else {
				assertHasFieldError(t, errs, "cpu")
			}
		})
	}
}

func TestCreateTier_MemoryFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"Mi", "512Mi", true},
		{"Gi", "1Gi", true},
		{"Ki", "1024Ki", true},
		{"bare number", "1024", true},
		{"invalid unit", "512MB", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.Memory = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "memory")
			} else {
				assertHasFieldError(t, errs, "memory")
			}
		})
	}
}

func TestCreateTier_StorageSizeFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"Gi", "1Gi", true},
		{"Mi", "500Mi", true},
		{"invalid unit", "1GB", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.StorageSize = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "storageSize")
			} else {
				assertHasFieldError(t, errs, "storageSize")
			}
		})
	}
}

func TestCreateTier_StorageClassMaxLength(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.StorageClass = strings.Repeat("a", 256)
	errs := validation.ValidateCreateTierRequest(req)
	assertHasFieldError(t, errs, "storageClass")
}

func TestCreateTier_DescriptionMaxLength(t *testing.T) {
	t.Parallel()
	req := validCreateTierRequest()
	req.Description = strings.Repeat("a", 1001)
	errs := validation.ValidateCreateTierRequest(req)
	assertHasFieldError(t, errs, "description")
}

func TestCreateTier_PGVersionEnum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"15", "15", true},
		{"16", "16", true},
		{"17", "17", true},
		{"14", "14", false},
		{"invalid", "latest", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.PGVersion = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "pgVersion")
			} else {
				assertHasFieldError(t, errs, "pgVersion")
			}
		})
	}
}

func TestCreateTier_PoolModeEnum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"empty", "", false},
		{"transaction", "transaction", true},
		{"session", "session", true},
		{"statement", "statement", true},
		{"invalid", "pool", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.PoolMode = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "poolMode")
			} else {
				assertHasFieldError(t, errs, "poolMode")
			}
		})
	}
}

func TestCreateTier_MaxConnectionsBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value int
		valid bool
	}{
		{"below min", 9, false},
		{"min", 10, true},
		{"mid", 500, true},
		{"max", 10000, true},
		{"above max", 10001, false},
		{"zero", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := validCreateTierRequest()
			req.MaxConnections = tt.value
			errs := validation.ValidateCreateTierRequest(req)
			if tt.valid {
				assertNoFieldError(t, errs, "maxConnections")
			} else {
				assertHasFieldError(t, errs, "maxConnections")
			}
		})
	}
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
	inst := 3
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Description: &desc,
		Instances:   &inst,
	})
	assert.Empty(t, errs)
}

func TestUpdateTier_InvalidInstances(t *testing.T) {
	t.Parallel()
	val := 0
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Instances: &val,
	})
	assertHasFieldError(t, errs, "instances")
}

func TestUpdateTier_EmptyCPU(t *testing.T) {
	t.Parallel()
	val := ""
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		CPU: &val,
	})
	assertHasFieldError(t, errs, "cpu")
}

func TestUpdateTier_InvalidMemory(t *testing.T) {
	t.Parallel()
	val := "512MB"
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Memory: &val,
	})
	assertHasFieldError(t, errs, "memory")
}

func TestUpdateTier_EmptyPGVersion(t *testing.T) {
	t.Parallel()
	val := ""
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		PGVersion: &val,
	})
	assertHasFieldError(t, errs, "pgVersion")
}

func TestUpdateTier_InvalidPoolMode(t *testing.T) {
	t.Parallel()
	val := "invalid"
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		PoolMode: &val,
	})
	assertHasFieldError(t, errs, "poolMode")
}

func TestUpdateTier_InvalidMaxConnections(t *testing.T) {
	t.Parallel()
	val := 5
	errs := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		MaxConnections: &val,
	})
	assertHasFieldError(t, errs, "maxConnections")
}

func TestUpdateTier_InvalidDestructionStrategy(t *testing.T) {
	t.Parallel()
	val := "drop"
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
