package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/config"
)

const testDatabaseURL = "postgres://user:pass@localhost:5432/daap_test?sslmode=disable"

func clearEnvVars(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "LOG_LEVEL", "DATABASE_URL", "KUBECONFIG_PATH", "NAMESPACE", "VERSION"} {
		os.Unsetenv(key)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("DATABASE_URL", testDatabaseURL)

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, testDatabaseURL, cfg.DatabaseURL)
	assert.Equal(t, "", cfg.KubeconfigPath)
	assert.Equal(t, "default", cfg.Namespace)
	assert.Equal(t, "dev", cfg.Version)
}

func TestLoad_EnvVarOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		assertFn func(t *testing.T, cfg *config.Config)
	}{
		{
			name:    "custom port",
			envVars: map[string]string{"PORT": "3000"},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, 3000, cfg.Port)
			},
		},
		{
			name:    "custom log level",
			envVars: map[string]string{"LOG_LEVEL": "debug"},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "debug", cfg.LogLevel)
			},
		},
		{
			name:    "custom kubeconfig path",
			envVars: map[string]string{"KUBECONFIG_PATH": "/home/user/.kube/config"},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "/home/user/.kube/config", cfg.KubeconfigPath)
			},
		},
		{
			name:    "custom namespace",
			envVars: map[string]string{"NAMESPACE": "production"},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "production", cfg.Namespace)
			},
		},
		{
			name:    "custom version",
			envVars: map[string]string{"VERSION": "1.2.3"},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "1.2.3", cfg.Version)
			},
		},
		{
			name: "all overrides at once",
			envVars: map[string]string{
				"PORT":            "9090",
				"LOG_LEVEL":       "error",
				"KUBECONFIG_PATH": "/etc/kube/config",
				"NAMESPACE":       "staging",
				"VERSION":         "2.0.0",
			},
			assertFn: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, 9090, cfg.Port)
				assert.Equal(t, "error", cfg.LogLevel)
				assert.Equal(t, "/etc/kube/config", cfg.KubeconfigPath)
				assert.Equal(t, "staging", cfg.Namespace)
				assert.Equal(t, "2.0.0", cfg.Version)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnvVars(t)
			t.Setenv("DATABASE_URL", testDatabaseURL)
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := config.Load()

			require.NoError(t, err)
			tt.assertFn(t, cfg)
		})
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	clearEnvVars(t)

	cfg, err := config.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnvVars(t)
	t.Setenv("DATABASE_URL", testDatabaseURL)
	t.Setenv("PORT", "not-a-number")

	cfg, err := config.Load()

	assert.Error(t, err)
	assert.Nil(t, cfg)
}
