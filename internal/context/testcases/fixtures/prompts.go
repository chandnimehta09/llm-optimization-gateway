// Package fixtures provides reusable test data for context identity tests.
package fixtures

// SamplePrompts contains sample prompt templates for testing.
var SamplePrompts = map[string]string{
	"code_review": `You are a code reviewer. Analyze the following code for bugs, security issues, and style problems.

Code:
{{.Code}}

Please provide a detailed review.`,

	"summarize": `Summarize the following text in {{.MaxSentences}} sentences or fewer.

Text:
{{.Text}}`,

	"translate": `Translate the following text from {{.SourceLang}} to {{.TargetLang}}.

Text:
{{.Text}}`,

	"empty": "",

	"minimal": "Hello",

	"large": generateLargePrompt(100000),
}

// SampleSystemPrompts contains sample system prompts for testing.
var SampleSystemPrompts = map[string]string{
	"default":    "You are a helpful assistant.",
	"code":       "You are an expert software engineer. You write clean, efficient, and well-documented code.",
	"empty":      "",
	"concise":    "Be concise.",
	"with_rules": "You are a helpful assistant. Follow these rules:\n1. Be accurate\n2. Cite sources\n3. Use formal language",
}

// generateLargePrompt creates a prompt of approximately the given byte size.
func generateLargePrompt(size int) string {
	base := "This is a test prompt line that is used to generate large context payloads for performance testing. "
	var result []byte
	for len(result) < size {
		result = append(result, base...)
	}
	return string(result[:size])
}
