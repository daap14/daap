package provider

import "sort"

// Registry maps provider names to Provider implementations.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry under the given name.
func (r *Registry) Register(name string, p Provider) {
	r.providers[name] = p
}

// Get returns the provider registered under the given name.
// Returns false if the name is not registered.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Has reports whether a provider with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.providers[name]
	return ok
}

// Names returns a sorted list of all registered provider names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
