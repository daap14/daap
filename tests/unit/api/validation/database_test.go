package validation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/daap14/daap/internal/api/validation"
)

func TestValidateName_Valid(t *testing.T) {
	tests := []string{
		"mydb",
		"my-database",
		"abc",
		"a1b2c3",
		"team-analytics-db",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
				Name:      name,
				OwnerTeam: "team-a",
				Tier:      "standard",
			})
			for _, e := range errs {
				assert.NotEqual(t, "name", e.Field, "expected no name error for %q, got: %s", name, e.Message)
			}
		})
	}
}

func TestValidateName_TooShort(t *testing.T) {
	tests := []string{"ab", "a", "x"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
				Name:      name,
				OwnerTeam: "team-a",
				Tier:      "standard",
			})
			hasNameErr := false
			for _, e := range errs {
				if e.Field == "name" {
					hasNameErr = true
					break
				}
			}
			assert.True(t, hasNameErr, "expected name validation error for %q", name)
		})
	}
}

func TestValidateName_TooLong(t *testing.T) {
	// 64 chars: starts with letter, all lowercase
	name := "a" + string(make([]byte, 63))
	for i := range name[1:] {
		_ = i
	}
	// Build a 64-char name properly
	longName := "a"
	for len(longName) < 64 {
		longName += "b"
	}

	errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
		Name:      longName,
		OwnerTeam: "team-a",
		Tier:      "standard",
	})
	hasNameErr := false
	for _, e := range errs {
		if e.Field == "name" {
			hasNameErr = true
			break
		}
	}
	assert.True(t, hasNameErr, "expected name validation error for 64-char name")
}

func TestValidateName_InvalidChars(t *testing.T) {
	tests := []string{
		"MyDatabase",
		"my_database",
		"my.database",
		"my database",
		"DB-NAME",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
				Name:      name,
				OwnerTeam: "team-a",
				Tier:      "standard",
			})
			hasNameErr := false
			for _, e := range errs {
				if e.Field == "name" {
					hasNameErr = true
					break
				}
			}
			assert.True(t, hasNameErr, "expected name validation error for %q", name)
		})
	}
}

func TestValidateName_MustStartWithLetter(t *testing.T) {
	tests := []string{
		"1database",
		"-database",
		"0abc",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
				Name:      name,
				OwnerTeam: "team-a",
				Tier:      "standard",
			})
			hasNameErr := false
			for _, e := range errs {
				if e.Field == "name" {
					hasNameErr = true
					break
				}
			}
			assert.True(t, hasNameErr, "expected name validation error for %q", name)
		})
	}
}

func TestValidateRequired_MissingName(t *testing.T) {
	errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
		Name:      "",
		OwnerTeam: "team-a",
		Tier:      "standard",
	})
	hasNameErr := false
	for _, e := range errs {
		if e.Field == "name" {
			hasNameErr = true
			assert.Contains(t, e.Message, "required")
			break
		}
	}
	assert.True(t, hasNameErr, "expected name required error")
}

func TestValidateRequired_MissingOwnerTeam(t *testing.T) {
	errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
		Name:      "mydb",
		OwnerTeam: "",
		Tier:      "standard",
	})
	hasOwnerErr := false
	for _, e := range errs {
		if e.Field == "ownerTeam" {
			hasOwnerErr = true
			assert.Contains(t, e.Message, "required")
			break
		}
	}
	assert.True(t, hasOwnerErr, "expected ownerTeam required error")
}

func TestValidateAll_MultipleErrors(t *testing.T) {
	errs := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
		Name:      "",
		OwnerTeam: "",
		Tier:      "",
	})

	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}

	assert.True(t, fields["name"], "expected name error")
	assert.True(t, fields["ownerTeam"], "expected ownerTeam error")
	assert.True(t, fields["tier"], "expected tier error")
	assert.GreaterOrEqual(t, len(errs), 3, "expected at least 3 field errors")
}
