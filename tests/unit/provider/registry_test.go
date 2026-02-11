package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/provider"
)

// fakeProvider is a no-op Provider for registry tests.
type fakeProvider struct{}

func (f *fakeProvider) Apply(_ context.Context, _ provider.ProviderDatabase, _ string) error {
	return nil
}
func (f *fakeProvider) Delete(_ context.Context, _ provider.ProviderDatabase) error {
	return nil
}
func (f *fakeProvider) CheckHealth(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
	return provider.HealthResult{Status: "ready"}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	fp := &fakeProvider{}
	reg.Register("cnpg", fp)

	got, ok := reg.Get("cnpg")
	require.True(t, ok)
	assert.Equal(t, fp, got)
}

func TestRegistry_GetUnknown(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()

	_, ok := reg.Get("rds")
	assert.False(t, ok)
}

func TestRegistry_Has(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	reg.Register("cnpg", &fakeProvider{})

	assert.True(t, reg.Has("cnpg"))
	assert.False(t, reg.Has("rds"))
}

func TestRegistry_Names(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	reg.Register("rds", &fakeProvider{})
	reg.Register("cnpg", &fakeProvider{})

	names := reg.Names()
	assert.Equal(t, []string{"cnpg", "rds"}, names) // sorted
}

func TestRegistry_NamesEmpty(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	names := reg.Names()
	assert.Empty(t, names)
}
