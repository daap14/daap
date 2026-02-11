package k8s

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client provides access to Kubernetes and CNPG resources.
type Client struct {
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
	config    *rest.Config
}

// ConnectivityStatus represents the result of a Kubernetes connectivity check.
type ConnectivityStatus struct {
	Connected bool
	Version   string
}

// ClientOption configures the Client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	kubeconfigPath string
}

// WithKubeconfig sets the kubeconfig file path for out-of-cluster access.
func WithKubeconfig(path string) ClientOption {
	return func(o *clientOptions) {
		o.kubeconfigPath = path
	}
}

// NewClient creates a new Kubernetes client.
// It attempts in-cluster configuration first, falling back to kubeconfig if provided.
func NewClient(opts ...ClientOption) (*Client, error) {
	o := &clientOptions{}
	for _, opt := range opts {
		opt(o)
	}

	cfg, err := buildConfig(o.kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("building kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating discovery client: %w", err)
	}

	return &Client{
		dynamic:   dynClient,
		discovery: disc,
		config:    cfg,
	}, nil
}

// DynamicClient returns the underlying dynamic Kubernetes client.
func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamic
}

// CheckConnectivity verifies that the Kubernetes API server is reachable
// and returns the server version. The ctx parameter is accepted to satisfy the
// HealthChecker interface and for future use when the discovery client supports
// context-aware calls.
func (c *Client) CheckConnectivity(ctx context.Context) ConnectivityStatus {
	info, err := c.discovery.ServerVersion()
	if err != nil {
		return ConnectivityStatus{Connected: false}
	}
	return ConnectivityStatus{
		Connected: true,
		Version:   info.GitVersion,
	}
}

// buildConfig creates a rest.Config, trying in-cluster first, then kubeconfig.
func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("loading kubeconfig from %s: %w", kubeconfigPath, err)
		}
		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	return nil, fmt.Errorf("no kubeconfig path provided and not running in-cluster: %w", err)
}
