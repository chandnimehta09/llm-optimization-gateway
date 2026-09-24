package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/llm-optimization-gateway/internal/request"
)

// Provider is the interface that LLM provider implementations must satisfy.
// It encapsulates sending a normalized request to a specific provider and
// returning the raw response.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string
	// Send sends a normalized request to the provider and returns the response.
	Send(ctx context.Context, req *request.NormalizedRequest) (*ProviderResponse, error)
}

// ProviderResponse is the response from a provider after executing a request.
type ProviderResponse struct {
	// StatusCode is the HTTP status code returned by the provider.
	StatusCode int `json:"status_code"`
	// Body is the raw response body.
	Body []byte `json:"body"`
	// Headers holds response headers.
	Headers map[string]string `json:"headers,omitempty"`
	// Model is the model that actually processed the request.
	Model string `json:"model,omitempty"`
	// Provider is the provider that processed the request.
	Provider string `json:"provider"`
	// InputTokens is the number of input tokens used.
	InputTokens int `json:"input_tokens,omitempty"`
	// OutputTokens is the number of output tokens generated.
	OutputTokens int `json:"output_tokens,omitempty"`
}

// providerEntry holds a registered provider with its capabilities.
type providerEntry struct {
	provider     Provider
	capabilities *ProviderCapabilities
}

// Registry stores provider instances, their capabilities, and health status.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*providerEntry
	health    *HealthChecker
}

// NewRegistry creates a new provider registry. If healthChecker is nil,
// no health checking is performed.
func NewRegistry(healthChecker *HealthChecker) *Registry {
	return &Registry{
		providers: make(map[string]*providerEntry),
		health:    healthChecker,
	}
}

// Register adds a provider to the registry with the given capabilities.
func (r *Registry) Register(provider Provider, capabilities *ProviderCapabilities) error {
	if provider == nil {
		return fmt.Errorf("registry: provider must not be nil")
	}
	name := provider.Name()
	if name == "" {
		return fmt.Errorf("registry: provider name must not be empty")
	}
	if capabilities == nil {
		return fmt.Errorf("registry: capabilities must not be nil for provider %q", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[name] = &providerEntry{
		provider:     provider,
		capabilities: capabilities,
	}

	if r.health != nil {
		r.health.RegisterProvider(name)
	}

	return nil
}

// Unregister removes a provider from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
	if r.health != nil {
		r.health.UnregisterProvider(name)
	}
}

// Get returns the provider with the given name, or an error if not found.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("registry: provider %q not found", name)
	}
	return entry.provider, nil
}

// GetCapabilities returns the capabilities for the named provider.
func (r *Registry) GetCapabilities(name string) (*ProviderCapabilities, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("registry: provider %q not found", name)
	}
	return entry.capabilities, nil
}

// IsHealthy returns whether the named provider is healthy. If no health checker
// is configured, it defaults to true for registered providers.
func (r *Registry) IsHealthy(name string) bool {
	r.mu.RLock()
	_, exists := r.providers[name]
	r.mu.RUnlock()
	if !exists {
		return false
	}
	if r.health == nil {
		return true
	}
	return r.health.IsHealthy(name)
}

// ListProviders returns the names of all registered providers.
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Size returns the number of registered providers.
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// FindProvidersForModel returns providers that support the given model.
func (r *Registry) FindProvidersForModel(model string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []string
	for name, entry := range r.providers {
		if entry.capabilities.SupportsModel(model) {
			result = append(result, name)
		}
	}
	return result
}

// FindProvidersWithFeature returns providers that support the given feature.
func (r *Registry) FindProvidersWithFeature(feature Feature) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []string
	for name, entry := range r.providers {
		if entry.capabilities.SupportsFeature(feature) {
			result = append(result, name)
		}
	}
	return result
}

// FindHealthyProviders returns the names of all healthy registered providers.
func (r *Registry) FindHealthyProviders() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	r.mu.RUnlock()

	var healthy []string
	for _, name := range names {
		if r.IsHealthy(name) {
			healthy = append(healthy, name)
		}
	}
	return healthy
}

// String returns a summary of the registry.
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("registry: %d providers", len(r.providers))
}
