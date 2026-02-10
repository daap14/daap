package config

import "github.com/kelseyhightower/envconfig"

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port               int    `envconfig:"PORT" default:"8080"`
	LogLevel           string `envconfig:"LOG_LEVEL" default:"info"`
	DatabaseURL        string `envconfig:"DATABASE_URL" required:"true"`
	KubeconfigPath     string `envconfig:"KUBECONFIG_PATH" default:""`
	Namespace          string `envconfig:"NAMESPACE" default:"default"`
	Version            string `envconfig:"VERSION" default:"dev"`
	ReconcilerInterval int    `envconfig:"RECONCILER_INTERVAL" default:"10"`
	BcryptCost         int    `envconfig:"BCRYPT_COST" default:"12"`
}

// Load reads configuration from environment variables into a Config struct.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
