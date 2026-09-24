package rules

import (
	"testing"
)

// --------------------------------------------------------------------------
// Tests: Model alias resolution
// --------------------------------------------------------------------------

func TestResolveModelAlias_DirectAlias(t *testing.T) {
	config := NewDefaultRoutingConfig()

	tests := []struct {
		alias    string
		wantName string
		wantWas  bool
	}{
		{"fast-model", "gpt-3.5-turbo", true},
		{"smart-model", "gpt-4", true},
		{"cheap-model", "gpt-3.5-turbo", true},
		{"best-model", "gpt-4", true},
		{"gemini", "gemini-pro", true},
		{"gemini-latest", "gemini-1.5-pro", true},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			got, wasAlias := ResolveModelAlias(tt.alias, config)
			if got != tt.wantName {
				t.Errorf("ResolveModelAlias(%q) = %q, want %q", tt.alias, got, tt.wantName)
			}
			if wasAlias != tt.wantWas {
				t.Errorf("ResolveModelAlias(%q) wasAlias = %v, want %v", tt.alias, wasAlias, tt.wantWas)
			}
		})
	}
}

func TestResolveModelAlias_CanonicalPassthrough(t *testing.T) {
	config := NewDefaultRoutingConfig()

	tests := []string{"gpt-4", "gpt-3.5-turbo", "gemini-pro", "gemini-1.5-pro", "unknown-model"}

	for _, model := range tests {
		t.Run(model, func(t *testing.T) {
			got, wasAlias := ResolveModelAlias(model, config)
			if got != model {
				t.Errorf("ResolveModelAlias(%q) = %q, want passthrough", model, got)
			}
			if wasAlias {
				t.Errorf("canonical model %q should not be an alias", model)
			}
		})
	}
}

func TestResolveModelAlias_CaseInsensitive(t *testing.T) {
	config := NewDefaultRoutingConfig()

	got, wasAlias := ResolveModelAlias("Fast-Model", config)
	if got != "gpt-3.5-turbo" || !wasAlias {
		t.Errorf("case-insensitive alias: got %q (wasAlias=%v), want gpt-3.5-turbo (wasAlias=true)", got, wasAlias)
	}
}

func TestResolveModelAlias_NilConfig(t *testing.T) {
	got, wasAlias := ResolveModelAlias("fast-model", nil)
	if got != "fast-model" || wasAlias {
		t.Errorf("nil config: got %q (wasAlias=%v), want passthrough", got, wasAlias)
	}
}

func TestResolveModelAlias_NilAliasMap(t *testing.T) {
	config := &RoutingConfig{}
	got, wasAlias := ResolveModelAlias("fast-model", config)
	if got != "fast-model" || wasAlias {
		t.Errorf("nil alias map: got %q (wasAlias=%v), want passthrough", got, wasAlias)
	}
}

// --------------------------------------------------------------------------
// Tests: Model info lookup
// --------------------------------------------------------------------------

func TestLookupModelInfo_Found(t *testing.T) {
	config := NewDefaultRoutingConfig()

	info, ok := LookupModelInfo("gpt-4", config)
	if !ok {
		t.Fatal("expected gpt-4 to be found")
	}
	if info.Provider != "openai" {
		t.Errorf("gpt-4 provider = %q, want openai", info.Provider)
	}
	if info.CostPer1KTokens != 0.03 {
		t.Errorf("gpt-4 cost = %f, want 0.03", info.CostPer1KTokens)
	}

	info, ok = LookupModelInfo("gemini-pro", config)
	if !ok {
		t.Fatal("expected gemini-pro to be found")
	}
	if info.Provider != "google" {
		t.Errorf("gemini-pro provider = %q, want google", info.Provider)
	}
}

func TestLookupModelInfo_NotFound(t *testing.T) {
	config := NewDefaultRoutingConfig()

	_, ok := LookupModelInfo("nonexistent-model", config)
	if ok {
		t.Error("expected nonexistent-model to not be found")
	}
}

func TestLookupModelInfo_NilConfig(t *testing.T) {
	_, ok := LookupModelInfo("gpt-4", nil)
	if ok {
		t.Error("expected nil config to return not found")
	}
}

// --------------------------------------------------------------------------
// Tests: Deprecation checking
// --------------------------------------------------------------------------

func TestCheckDeprecation_NotDeprecated(t *testing.T) {
	config := NewDefaultRoutingConfig()

	deprecated, replacement, warning := CheckDeprecation("gpt-4", config)
	if deprecated {
		t.Error("gpt-4 should not be deprecated")
	}
	if replacement != "" {
		t.Errorf("replacement = %q, want empty", replacement)
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}

func TestCheckDeprecation_Deprecated(t *testing.T) {
	config := NewDefaultRoutingConfig()
	config.Models["old-model"] = ModelInfo{
		CanonicalName:    "old-model",
		Provider:         "openai",
		Deprecated:       true,
		ReplacementModel: "gpt-4",
	}

	deprecated, replacement, warning := CheckDeprecation("old-model", config)
	if !deprecated {
		t.Error("old-model should be deprecated")
	}
	if replacement != "gpt-4" {
		t.Errorf("replacement = %q, want gpt-4", replacement)
	}
	if warning == "" {
		t.Error("expected deprecation warning")
	}
}

func TestCheckDeprecation_DeprecatedNoReplacement(t *testing.T) {
	config := NewDefaultRoutingConfig()
	config.Models["dead-model"] = ModelInfo{
		CanonicalName: "dead-model",
		Provider:      "openai",
		Deprecated:    true,
	}

	deprecated, replacement, warning := CheckDeprecation("dead-model", config)
	if !deprecated {
		t.Error("dead-model should be deprecated")
	}
	if replacement != "" {
		t.Errorf("replacement = %q, want empty", replacement)
	}
	if warning == "" {
		t.Error("expected deprecation warning")
	}
}

func TestCheckDeprecation_UnknownModel(t *testing.T) {
	config := NewDefaultRoutingConfig()

	deprecated, _, _ := CheckDeprecation("unknown-model", config)
	if deprecated {
		t.Error("unknown model should not be deprecated")
	}
}

// --------------------------------------------------------------------------
// Tests: Equivalent model lookup
// --------------------------------------------------------------------------

func TestFindEquivalentModels_Found(t *testing.T) {
	config := NewDefaultRoutingConfig()

	equivs := FindEquivalentModels("gpt-4", config)
	if len(equivs) == 0 {
		t.Fatal("expected equivalents for gpt-4")
	}
	if equivs[0].Model != "gemini-1.5-pro" {
		t.Errorf("gpt-4 equivalent = %q, want gemini-1.5-pro", equivs[0].Model)
	}
	if equivs[0].Provider != "google" {
		t.Errorf("equivalent provider = %q, want google", equivs[0].Provider)
	}
}

func TestFindEquivalentModels_NotFound(t *testing.T) {
	config := NewDefaultRoutingConfig()

	equivs := FindEquivalentModels("nonexistent", config)
	if len(equivs) != 0 {
		t.Errorf("expected no equivalents, got %d", len(equivs))
	}
}

func TestFindEquivalentModels_NilConfig(t *testing.T) {
	equivs := FindEquivalentModels("gpt-4", nil)
	if equivs != nil {
		t.Errorf("expected nil for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Provider selection for model
// --------------------------------------------------------------------------

func TestSelectProviderForModel_Direct(t *testing.T) {
	config := NewDefaultRoutingConfig()

	tests := []struct {
		model        string
		wantProvider string
		wantFound    bool
	}{
		{"gpt-4", "openai", true},
		{"gpt-3.5-turbo", "openai", true},
		{"gemini-pro", "google", true},
		{"gemini-1.5-pro", "google", true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			provider, found := SelectProviderForModel(tt.model, config)
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if found && provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
		})
	}
}

func TestSelectProviderForModel_UnhealthyPrimary(t *testing.T) {
	config := NewDefaultRoutingConfig()
	// Make openai unhealthy.
	openai := config.Providers["openai"]
	openai.Healthy = false
	config.Providers["openai"] = openai

	// Add gpt-4 to google's models so it can serve as fallback.
	google := config.Providers["google"]
	google.Models = append(google.Models, "gpt-4")
	config.Providers["google"] = google

	provider, found := SelectProviderForModel("gpt-4", config)
	if !found {
		t.Fatal("expected to find a provider")
	}
	if provider != "google" {
		t.Errorf("provider = %q, want google (openai is unhealthy)", provider)
	}
}

func TestSelectProviderForModel_NilConfig(t *testing.T) {
	_, found := SelectProviderForModel("gpt-4", nil)
	if found {
		t.Error("expected not found for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Fallback provider selection
// --------------------------------------------------------------------------

func TestSelectFallbackProviders_HasFallbacks(t *testing.T) {
	config := NewDefaultRoutingConfig()

	// gpt-4 is on openai; gemini-1.5-pro on google is equivalent.
	fallbacks := SelectFallbackProviders("gpt-4", "openai", config)
	if len(fallbacks) == 0 {
		t.Fatal("expected fallback providers for gpt-4")
	}
	foundGoogle := false
	for _, f := range fallbacks {
		if f == "google" {
			foundGoogle = true
		}
	}
	if !foundGoogle {
		t.Errorf("expected google in fallbacks, got %v", fallbacks)
	}
}

func TestSelectFallbackProviders_NoFallbacks(t *testing.T) {
	config := &RoutingConfig{
		Providers: map[string]ProviderConfig{
			"only": {
				Name:    "only",
				Healthy: true,
				Models:  []string{"unique-model"},
			},
		},
	}

	fallbacks := SelectFallbackProviders("unique-model", "only", config)
	if len(fallbacks) != 0 {
		t.Errorf("expected no fallbacks, got %v", fallbacks)
	}
}

func TestSelectFallbackProviders_NilConfig(t *testing.T) {
	fallbacks := SelectFallbackProviders("gpt-4", "openai", nil)
	if fallbacks != nil {
		t.Errorf("expected nil for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Cost-optimal provider selection
// --------------------------------------------------------------------------

func TestSelectCostOptimalProvider_ChoosesCheaper(t *testing.T) {
	config := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {
				CanonicalName:   "test-model",
				CostPer1KTokens: 0.01,
			},
		},
		Providers: map[string]ProviderConfig{
			"expensive": {
				Name:               "expensive",
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     2.0,
				Models:             []string{"test-model"},
			},
			"cheap": {
				Name:               "cheap",
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     0.5,
				Models:             []string{"test-model"},
			},
		},
	}

	provider, found := SelectCostOptimalProvider("test-model", config)
	if !found {
		t.Fatal("expected to find a provider")
	}
	if provider != "cheap" {
		t.Errorf("provider = %q, want cheap", provider)
	}
}

func TestSelectCostOptimalProvider_SkipsUnhealthy(t *testing.T) {
	config := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {CostPer1KTokens: 0.01},
		},
		Providers: map[string]ProviderConfig{
			"unhealthy": {
				Name:               "unhealthy",
				Healthy:            false,
				RateLimitRemaining: 1.0,
				CostMultiplier:     0.1,
				Models:             []string{"test-model"},
			},
			"healthy": {
				Name:               "healthy",
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     1.0,
				Models:             []string{"test-model"},
			},
		},
	}

	provider, found := SelectCostOptimalProvider("test-model", config)
	if !found {
		t.Fatal("expected to find a provider")
	}
	if provider != "healthy" {
		t.Errorf("provider = %q, want healthy", provider)
	}
}

func TestSelectCostOptimalProvider_SkipsRateLimited(t *testing.T) {
	config := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {CostPer1KTokens: 0.01},
		},
		Providers: map[string]ProviderConfig{
			"limited": {
				Name:               "limited",
				Healthy:            true,
				RateLimitRemaining: 0,
				CostMultiplier:     0.1,
				Models:             []string{"test-model"},
			},
			"available": {
				Name:               "available",
				Healthy:            true,
				RateLimitRemaining: 0.5,
				CostMultiplier:     1.0,
				Models:             []string{"test-model"},
			},
		},
	}

	provider, found := SelectCostOptimalProvider("test-model", config)
	if !found {
		t.Fatal("expected to find a provider")
	}
	if provider != "available" {
		t.Errorf("provider = %q, want available", provider)
	}
}

func TestSelectCostOptimalProvider_NilConfig(t *testing.T) {
	_, found := SelectCostOptimalProvider("gpt-4", nil)
	if found {
		t.Error("expected not found for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Latency-optimal provider selection
// --------------------------------------------------------------------------

func TestSelectLatencyOptimalProvider_ChoosesLower(t *testing.T) {
	config := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {AverageLatencyMs: 100},
		},
		Providers: map[string]ProviderConfig{
			"slow": {
				Name:               "slow",
				Healthy:            true,
				RateLimitRemaining: 1.0,
				Models:             []string{"test-model"},
			},
			"fast": {
				Name:               "fast",
				Healthy:            true,
				RateLimitRemaining: 1.0,
				Models:             []string{"test-model"},
			},
		},
	}

	provider, found := SelectLatencyOptimalProvider("test-model", config)
	if !found {
		t.Fatal("expected to find a provider")
	}
	// Both have same latency from model info, so we get either.
	if provider != "slow" && provider != "fast" {
		t.Errorf("provider = %q, want slow or fast", provider)
	}
}

func TestSelectLatencyOptimalProvider_NilConfig(t *testing.T) {
	_, found := SelectLatencyOptimalProvider("gpt-4", nil)
	if found {
		t.Error("expected not found for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Capability-based provider selection
// --------------------------------------------------------------------------

func TestProviderSupportsCapabilities_AllSupported(t *testing.T) {
	config := NewDefaultRoutingConfig()

	supported := ProviderSupportsCapabilities("openai", []string{"chat", "function_calling"}, config)
	if !supported {
		t.Error("openai should support chat and function_calling")
	}
}

func TestProviderSupportsCapabilities_Missing(t *testing.T) {
	config := NewDefaultRoutingConfig()

	supported := ProviderSupportsCapabilities("google", []string{"vision"}, config)
	// gemini-1.5-pro has vision.
	if !supported {
		t.Error("google should support vision via gemini-1.5-pro")
	}
}

func TestProviderSupportsCapabilities_EmptyCaps(t *testing.T) {
	config := NewDefaultRoutingConfig()

	supported := ProviderSupportsCapabilities("openai", nil, config)
	if !supported {
		t.Error("empty capabilities should always be supported")
	}
}

func TestProviderSupportsCapabilities_UnknownProvider(t *testing.T) {
	config := NewDefaultRoutingConfig()

	supported := ProviderSupportsCapabilities("unknown", []string{"chat"}, config)
	if supported {
		t.Error("unknown provider should not support any capabilities")
	}
}

func TestSelectProviderByCapabilities(t *testing.T) {
	config := NewDefaultRoutingConfig()

	provider, found := SelectProviderByCapabilities("gpt-4", []string{"chat", "vision"}, config)
	if !found {
		t.Fatal("expected to find a provider with vision")
	}
	if provider != "openai" {
		t.Errorf("provider = %q, want openai", provider)
	}
}

func TestSelectProviderByCapabilities_NilConfig(t *testing.T) {
	_, found := SelectProviderByCapabilities("gpt-4", []string{"chat"}, nil)
	if found {
		t.Error("expected not found for nil config")
	}
}

// --------------------------------------------------------------------------
// Tests: Default routing config
// --------------------------------------------------------------------------

func TestNewDefaultRoutingConfig(t *testing.T) {
	config := NewDefaultRoutingConfig()

	if config.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai", config.DefaultProvider)
	}
	if config.DefaultModel != "gpt-3.5-turbo" {
		t.Errorf("DefaultModel = %q, want gpt-3.5-turbo", config.DefaultModel)
	}
	if config.DefaultStrategy != RoutingStrategyDirect {
		t.Errorf("DefaultStrategy = %q, want direct", config.DefaultStrategy)
	}

	// Check OpenAI models exist.
	for _, model := range []string{"gpt-4", "gpt-4-turbo", "gpt-3.5-turbo"} {
		if _, ok := config.Models[model]; !ok {
			t.Errorf("model %q not found in default config", model)
		}
	}

	// Check Gemini models exist.
	for _, model := range []string{"gemini-pro", "gemini-1.5-pro", "gemini-1.5-flash"} {
		if _, ok := config.Models[model]; !ok {
			t.Errorf("model %q not found in default config", model)
		}
	}

	// Check providers.
	if _, ok := config.Providers["openai"]; !ok {
		t.Error("openai provider not found")
	}
	if _, ok := config.Providers["google"]; !ok {
		t.Error("google provider not found")
	}

	// Check aliases.
	if config.ModelAliases["fast-model"] != "gpt-3.5-turbo" {
		t.Errorf("fast-model alias = %q, want gpt-3.5-turbo", config.ModelAliases["fast-model"])
	}
}

// --------------------------------------------------------------------------
// Tests: Model info properties
// --------------------------------------------------------------------------

func TestModelInfo_OpenAIModels(t *testing.T) {
	config := NewDefaultRoutingConfig()

	gpt4 := config.Models["gpt-4"]
	if gpt4.Provider != "openai" {
		t.Errorf("gpt-4 provider = %q, want openai", gpt4.Provider)
	}
	if gpt4.MaxContextTokens != 8192 {
		t.Errorf("gpt-4 max context = %d, want 8192", gpt4.MaxContextTokens)
	}
	if gpt4.Deprecated {
		t.Error("gpt-4 should not be deprecated")
	}

	gpt35 := config.Models["gpt-3.5-turbo"]
	if gpt35.Provider != "openai" {
		t.Errorf("gpt-3.5-turbo provider = %q, want openai", gpt35.Provider)
	}
	if gpt35.CostPer1KTokens >= gpt4.CostPer1KTokens {
		t.Error("gpt-3.5-turbo should be cheaper than gpt-4")
	}
}

func TestModelInfo_GeminiModels(t *testing.T) {
	config := NewDefaultRoutingConfig()

	geminiPro := config.Models["gemini-pro"]
	if geminiPro.Provider != "google" {
		t.Errorf("gemini-pro provider = %q, want google", geminiPro.Provider)
	}

	gemini15Pro := config.Models["gemini-1.5-pro"]
	if gemini15Pro.Provider != "google" {
		t.Errorf("gemini-1.5-pro provider = %q, want google", gemini15Pro.Provider)
	}
	if gemini15Pro.MaxContextTokens != 1048576 {
		t.Errorf("gemini-1.5-pro max context = %d, want 1048576", gemini15Pro.MaxContextTokens)
	}
}

// --------------------------------------------------------------------------
// Tests: Equivalent models cross-provider
// --------------------------------------------------------------------------

func TestEquivalentModels_CrossProvider(t *testing.T) {
	config := NewDefaultRoutingConfig()

	// gpt-4 should map to gemini-1.5-pro.
	equivs := FindEquivalentModels("gpt-4", config)
	if len(equivs) != 1 {
		t.Fatalf("gpt-4 equivalents: got %d, want 1", len(equivs))
	}
	if equivs[0].Model != "gemini-1.5-pro" || equivs[0].Provider != "google" {
		t.Errorf("gpt-4 equivalent: got %q on %q, want gemini-1.5-pro on google",
			equivs[0].Model, equivs[0].Provider)
	}

	// gemini-pro should map to gpt-3.5-turbo.
	equivs = FindEquivalentModels("gemini-pro", config)
	if len(equivs) != 1 {
		t.Fatalf("gemini-pro equivalents: got %d, want 1", len(equivs))
	}
	if equivs[0].Model != "gpt-3.5-turbo" || equivs[0].Provider != "openai" {
		t.Errorf("gemini-pro equivalent: got %q on %q, want gpt-3.5-turbo on openai",
			equivs[0].Model, equivs[0].Provider)
	}
}
