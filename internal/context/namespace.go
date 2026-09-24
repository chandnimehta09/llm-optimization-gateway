package context

import (
	"errors"
	"fmt"
	"strings"
)

// NamespaceType classifies the kind of namespace applied to cache keys.
type NamespaceType string

const (
	// NamespaceTenant scopes cache keys to a specific customer or tenant.
	NamespaceTenant NamespaceType = "tenant"
	// NamespaceEnvironment scopes cache keys to a deployment environment.
	NamespaceEnvironment NamespaceType = "env"
	// NamespaceApplication scopes cache keys to a specific application.
	NamespaceApplication NamespaceType = "app"
)

// ValidEnvironments enumerates recognized environment names.
var ValidEnvironments = map[string]bool{
	"dev":        true,
	"staging":    true,
	"prod":       true,
	"test":       true,
	"production": true,
}

// Namespace represents a scoping context applied as a prefix to cache keys.
// Multiple namespace dimensions can be combined.
type Namespace struct {
	// Tenant is the customer/tenant identifier.
	Tenant string `json:"tenant,omitempty"`
	// Environment is the deployment environment (dev, staging, prod).
	Environment string `json:"environment,omitempty"`
	// Application is the application identifier.
	Application string `json:"application,omitempty"`
}

// NamespaceOption is a functional option for configuring a Namespace.
type NamespaceOption func(*Namespace)

// WithTenant sets the tenant namespace dimension.
func WithTenant(tenant string) NamespaceOption {
	return func(ns *Namespace) {
		ns.Tenant = tenant
	}
}

// WithEnvironment sets the environment namespace dimension.
func WithEnvironment(env string) NamespaceOption {
	return func(ns *Namespace) {
		ns.Environment = env
	}
}

// WithApplication sets the application namespace dimension.
func WithApplication(app string) NamespaceOption {
	return func(ns *Namespace) {
		ns.Application = app
	}
}

// NewNamespace creates a Namespace with the given options.
func NewNamespace(opts ...NamespaceOption) *Namespace {
	ns := &Namespace{}
	for _, opt := range opts {
		opt(ns)
	}
	return ns
}

// Validate checks that the namespace is well-formed.
// At least one dimension must be set, and if an environment is specified
// it must be one of the recognized values.
func (ns *Namespace) Validate() error {
	if ns == nil {
		return errors.New("namespace must not be nil")
	}
	if ns.Tenant == "" && ns.Environment == "" && ns.Application == "" {
		return errors.New("namespace must have at least one dimension set")
	}
	if ns.Environment != "" && !ValidEnvironments[ns.Environment] {
		return fmt.Errorf("invalid environment %q: must be one of dev, staging, prod, test, production", ns.Environment)
	}
	if ns.Tenant != "" && strings.Contains(ns.Tenant, KeySeparator) {
		return fmt.Errorf("tenant %q must not contain the key separator %q", ns.Tenant, KeySeparator)
	}
	if ns.Application != "" && strings.Contains(ns.Application, KeySeparator) {
		return fmt.Errorf("application %q must not contain the key separator %q", ns.Application, KeySeparator)
	}
	return nil
}

// Prefix returns the namespace string for use as a cache key prefix.
// The format is: [tenant:<tenant>:][env:<env>:][app:<app>]
// Only set dimensions are included.
func (ns *Namespace) Prefix() string {
	if ns == nil {
		return ""
	}

	var parts []string
	if ns.Tenant != "" {
		parts = append(parts, string(NamespaceTenant), ns.Tenant)
	}
	if ns.Environment != "" {
		parts = append(parts, string(NamespaceEnvironment), ns.Environment)
	}
	if ns.Application != "" {
		parts = append(parts, string(NamespaceApplication), ns.Application)
	}
	return strings.Join(parts, KeySeparator)
}

// String returns a human-readable representation of the namespace.
func (ns *Namespace) String() string {
	if ns == nil {
		return "<nil>"
	}
	prefix := ns.Prefix()
	if prefix == "" {
		return "<empty>"
	}
	return prefix
}

// IsEmpty returns true if no namespace dimensions are set.
func (ns *Namespace) IsEmpty() bool {
	if ns == nil {
		return true
	}
	return ns.Tenant == "" && ns.Environment == "" && ns.Application == ""
}

// Equal returns true if two namespaces have identical dimensions.
func (ns *Namespace) Equal(other *Namespace) bool {
	if ns == nil && other == nil {
		return true
	}
	if ns == nil || other == nil {
		return false
	}
	return ns.Tenant == other.Tenant &&
		ns.Environment == other.Environment &&
		ns.Application == other.Application
}

// ParseNamespacePrefix parses a namespace prefix string back into a Namespace.
// The prefix format is: [tenant:<tenant>:][env:<env>:][app:<app>]
func ParseNamespacePrefix(prefix string) (*Namespace, error) {
	if prefix == "" {
		return nil, errors.New("namespace prefix must not be empty")
	}

	parts := strings.Split(prefix, KeySeparator)
	ns := &Namespace{}

	i := 0
	for i < len(parts)-1 {
		key := parts[i]
		value := parts[i+1]
		switch NamespaceType(key) {
		case NamespaceTenant:
			ns.Tenant = value
			i += 2
		case NamespaceEnvironment:
			ns.Environment = value
			i += 2
		case NamespaceApplication:
			ns.Application = value
			i += 2
		default:
			return nil, fmt.Errorf("unknown namespace type %q at position %d", key, i)
		}
	}

	// If there's a trailing unpaired element, it's malformed.
	if i < len(parts) {
		return nil, fmt.Errorf("malformed namespace prefix: trailing segment %q", parts[i])
	}

	if ns.IsEmpty() {
		return nil, errors.New("no namespace dimensions found in prefix")
	}

	return ns, nil
}

// MergeNamespaces combines two namespaces, with the override taking precedence
// for any dimension that is set in both.
func MergeNamespaces(base, override *Namespace) *Namespace {
	if base == nil && override == nil {
		return nil
	}

	result := &Namespace{}

	if base != nil {
		result.Tenant = base.Tenant
		result.Environment = base.Environment
		result.Application = base.Application
	}

	if override != nil {
		if override.Tenant != "" {
			result.Tenant = override.Tenant
		}
		if override.Environment != "" {
			result.Environment = override.Environment
		}
		if override.Application != "" {
			result.Application = override.Application
		}
	}

	return result
}
