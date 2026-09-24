package context

import (
	"testing"
)

func TestNamespace_Prefix(t *testing.T) {
	tests := []struct {
		name     string
		ns       *Namespace
		expected string
	}{
		{
			name:     "tenant only",
			ns:       NewNamespace(WithTenant("acme")),
			expected: "tenant:acme",
		},
		{
			name:     "environment only",
			ns:       NewNamespace(WithEnvironment("prod")),
			expected: "env:prod",
		},
		{
			name:     "application only",
			ns:       NewNamespace(WithApplication("myapp")),
			expected: "app:myapp",
		},
		{
			name:     "tenant and environment",
			ns:       NewNamespace(WithTenant("acme"), WithEnvironment("staging")),
			expected: "tenant:acme:env:staging",
		},
		{
			name:     "all dimensions",
			ns:       NewNamespace(WithTenant("acme"), WithEnvironment("prod"), WithApplication("api")),
			expected: "tenant:acme:env:prod:app:api",
		},
		{
			name:     "nil namespace",
			ns:       nil,
			expected: "",
		},
		{
			name:     "empty namespace",
			ns:       NewNamespace(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ns.Prefix()
			if got != tt.expected {
				t.Errorf("expected prefix %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestNamespace_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ns      *Namespace
		wantErr bool
	}{
		{
			name:    "valid tenant",
			ns:      NewNamespace(WithTenant("acme")),
			wantErr: false,
		},
		{
			name:    "valid environment",
			ns:      NewNamespace(WithEnvironment("prod")),
			wantErr: false,
		},
		{
			name:    "valid application",
			ns:      NewNamespace(WithApplication("myapp")),
			wantErr: false,
		},
		{
			name:    "valid all dimensions",
			ns:      NewNamespace(WithTenant("acme"), WithEnvironment("dev"), WithApplication("api")),
			wantErr: false,
		},
		{
			name:    "invalid environment",
			ns:      NewNamespace(WithEnvironment("banana")),
			wantErr: true,
		},
		{
			name:    "empty namespace",
			ns:      NewNamespace(),
			wantErr: true,
		},
		{
			name:    "nil namespace",
			ns:      nil,
			wantErr: true,
		},
		{
			name:    "tenant with colon",
			ns:      NewNamespace(WithTenant("acme:corp")),
			wantErr: true,
		},
		{
			name:    "application with colon",
			ns:      NewNamespace(WithApplication("my:app")),
			wantErr: true,
		},
		{
			name:    "valid test environment",
			ns:      NewNamespace(WithEnvironment("test")),
			wantErr: false,
		},
		{
			name:    "valid production environment",
			ns:      NewNamespace(WithEnvironment("production")),
			wantErr: false,
		},
		{
			name:    "valid staging environment",
			ns:      NewNamespace(WithEnvironment("staging")),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ns.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestNamespace_IsEmpty(t *testing.T) {
	if !(*Namespace)(nil).IsEmpty() {
		t.Error("nil namespace should be empty")
	}
	if !NewNamespace().IsEmpty() {
		t.Error("empty namespace should be empty")
	}
	if NewNamespace(WithTenant("acme")).IsEmpty() {
		t.Error("namespace with tenant should not be empty")
	}
}

func TestNamespace_Equal(t *testing.T) {
	ns1 := NewNamespace(WithTenant("acme"), WithEnvironment("prod"))
	ns2 := NewNamespace(WithTenant("acme"), WithEnvironment("prod"))
	ns3 := NewNamespace(WithTenant("globex"), WithEnvironment("prod"))

	if !ns1.Equal(ns2) {
		t.Error("identical namespaces should be equal")
	}
	if ns1.Equal(ns3) {
		t.Error("different namespaces should not be equal")
	}
	if ns1.Equal(nil) {
		t.Error("non-nil should not equal nil")
	}
	if (*Namespace)(nil).Equal(ns1) {
		t.Error("nil should not equal non-nil")
	}
	if !(*Namespace)(nil).Equal(nil) {
		t.Error("nil should equal nil")
	}
}

func TestNamespace_String(t *testing.T) {
	ns := NewNamespace(WithTenant("acme"))
	if ns.String() != "tenant:acme" {
		t.Errorf("expected %q, got %q", "tenant:acme", ns.String())
	}

	empty := NewNamespace()
	if empty.String() != "<empty>" {
		t.Errorf("expected %q, got %q", "<empty>", empty.String())
	}

	if (*Namespace)(nil).String() != "<nil>" {
		t.Errorf("expected %q, got %q", "<nil>", (*Namespace)(nil).String())
	}
}

func TestParseNamespacePrefix(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		wantTenant string
		wantEnv    string
		wantApp    string
		wantErr    bool
	}{
		{
			name:       "tenant only",
			prefix:     "tenant:acme",
			wantTenant: "acme",
		},
		{
			name:    "environment only",
			prefix:  "env:prod",
			wantEnv: "prod",
		},
		{
			name:    "application only",
			prefix:  "app:myapp",
			wantApp: "myapp",
		},
		{
			name:       "all dimensions",
			prefix:     "tenant:acme:env:prod:app:api",
			wantTenant: "acme",
			wantEnv:    "prod",
			wantApp:    "api",
		},
		{
			name:    "empty prefix",
			prefix:  "",
			wantErr: true,
		},
		{
			name:    "unknown type",
			prefix:  "unknown:value",
			wantErr: true,
		},
		{
			name:    "odd number of segments",
			prefix:  "tenant:acme:orphan",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := ParseNamespacePrefix(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNamespacePrefix(%q) error = %v, wantErr = %v", tt.prefix, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if ns.Tenant != tt.wantTenant {
				t.Errorf("tenant: expected %q, got %q", tt.wantTenant, ns.Tenant)
			}
			if ns.Environment != tt.wantEnv {
				t.Errorf("environment: expected %q, got %q", tt.wantEnv, ns.Environment)
			}
			if ns.Application != tt.wantApp {
				t.Errorf("application: expected %q, got %q", tt.wantApp, ns.Application)
			}
		})
	}
}

func TestParseNamespacePrefix_RoundTrip(t *testing.T) {
	original := NewNamespace(WithTenant("acme"), WithEnvironment("staging"), WithApplication("backend"))
	prefix := original.Prefix()

	parsed, err := ParseNamespacePrefix(prefix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !original.Equal(parsed) {
		t.Errorf("round-trip failed: %v != %v", original, parsed)
	}
}

func TestMergeNamespaces(t *testing.T) {
	base := NewNamespace(WithTenant("acme"), WithEnvironment("dev"))
	override := NewNamespace(WithEnvironment("prod"), WithApplication("api"))

	merged := MergeNamespaces(base, override)
	if merged.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", merged.Tenant)
	}
	if merged.Environment != "prod" {
		t.Errorf("expected environment %q, got %q", "prod", merged.Environment)
	}
	if merged.Application != "api" {
		t.Errorf("expected application %q, got %q", "api", merged.Application)
	}
}

func TestMergeNamespaces_NilInputs(t *testing.T) {
	result := MergeNamespaces(nil, nil)
	if result != nil {
		t.Error("merging two nils should return nil")
	}

	ns := NewNamespace(WithTenant("acme"))
	result = MergeNamespaces(nil, ns)
	if result.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", result.Tenant)
	}

	result = MergeNamespaces(ns, nil)
	if result.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", result.Tenant)
	}
}
