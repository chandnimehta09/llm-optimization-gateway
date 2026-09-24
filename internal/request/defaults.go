package request

import "strings"

// Default parameter values applied during normalization.
const (
	DefaultTemperature = 1.0
	DefaultTopP        = 1.0
)

// ModelAlias maps common model aliases to their canonical names.
// This is configurable; the default set covers common cases.
var ModelAlias = map[string]string{
	"gpt-4":            "gpt-4",
	"gpt-4-turbo":      "gpt-4-turbo",
	"gpt-4o":           "gpt-4o",
	"gpt-4o-mini":      "gpt-4o-mini",
	"gpt-3.5-turbo":    "gpt-3.5-turbo",
	"gpt-3.5":          "gpt-3.5-turbo",
	"gemini-pro":       "gemini-pro",
	"gemini-1.5-pro":   "gemini-1.5-pro",
	"gemini-1.5-flash": "gemini-1.5-flash",
	"gemini-2.0-flash": "gemini-2.0-flash",
	"gemini-ultra":     "gemini-ultra",
}

// ProviderPrefixes are known provider prefixes that may appear before model names.
var ProviderPrefixes = []string{
	"openai/",
	"google/",
	"gemini/",
	"anthropic/",
	"azure/",
}

// GeminiModels is the set of model name prefixes that indicate a Gemini model.
var GeminiModels = []string{
	"gemini-",
	"models/gemini-",
}

// OpenAIModels is the set of model name prefixes that indicate an OpenAI model.
var OpenAIModels = []string{
	"gpt-",
	"o1-",
	"o3-",
	"chatgpt-",
	"ft:gpt-",
}

// DeprecatedParams maps deprecated parameter names to their replacements.
var DeprecatedParams = map[string]string{
	"max_tokens": "max_tokens", // still valid but included for completeness
}

// NormalizerConfig holds configurable settings for the normalizer.
type NormalizerConfig struct {
	// ModelAliases maps alias names to canonical model names.
	ModelAliases map[string]string
	// ApplyDefaultTemperature controls whether to set temperature if missing.
	ApplyDefaultTemperature bool
	// ApplyDefaultTopP controls whether to set top_p if missing.
	ApplyDefaultTopP bool
	// DefaultTemperature is the default temperature value.
	DefaultTemperature float64
	// DefaultTopP is the default top_p value.
	DefaultTopP float64
}

// DefaultNormalizerConfig returns the default normalizer configuration.
func DefaultNormalizerConfig() *NormalizerConfig {
	aliases := make(map[string]string, len(ModelAlias))
	for k, v := range ModelAlias {
		aliases[k] = v
	}
	return &NormalizerConfig{
		ModelAliases:            aliases,
		ApplyDefaultTemperature: true,
		ApplyDefaultTopP:        true,
		DefaultTemperature:      DefaultTemperature,
		DefaultTopP:             DefaultTopP,
	}
}

// StripProviderPrefix removes a known provider prefix from a model name
// and returns the cleaned model name plus the detected provider hint.
func StripProviderPrefix(model string) (cleanModel string, providerHint string) {
	for _, prefix := range ProviderPrefixes {
		if strings.HasPrefix(model, prefix) {
			cleanModel = strings.TrimPrefix(model, prefix)
			providerHint = strings.TrimSuffix(prefix, "/")
			return cleanModel, providerHint
		}
	}
	return model, ""
}

// ResolveModelAlias maps a model name to its canonical form using the
// provided alias map. If no alias exists, the name is returned as-is.
func ResolveModelAlias(model string, aliases map[string]string) string {
	if canonical, ok := aliases[model]; ok {
		return canonical
	}
	return model
}

// DetectProviderFromModel determines the provider hint based on model name
// patterns when no explicit provider prefix was present.
func DetectProviderFromModel(model string) string {
	lower := strings.ToLower(model)
	for _, prefix := range GeminiModels {
		if strings.HasPrefix(lower, prefix) {
			return "gemini"
		}
	}
	for _, prefix := range OpenAIModels {
		if strings.HasPrefix(lower, prefix) {
			return "openai"
		}
	}
	return ""
}
