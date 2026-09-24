package workloads

import (
	"fmt"
	"strings"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// RepositoryContextGenerator produces requests that simulate large repository
// contexts with file contents, mimicking IDE-style code completion scenarios.
type RepositoryContextGenerator struct {
	numFiles     int
	fileTokens   int
	files        []string
	instructions []string
}

// NewRepositoryContextGenerator creates a repository context workload generator.
func NewRepositoryContextGenerator(cfg benchmark.WorkloadConfig) *RepositoryContextGenerator {
	numFiles := paramInt(cfg.Params, "num_files", 5)
	fileTokens := paramInt(cfg.Params, "file_tokens", 500)

	files := make([]string, numFiles)
	for i := 0; i < numFiles; i++ {
		files[i] = generateRepoFile(i, fileTokens)
	}

	instructions := []string{
		"Add error handling to the main function.",
		"Write unit tests for the handler functions.",
		"Refactor the database connection to use connection pooling.",
		"Add logging to all API endpoints.",
		"Implement rate limiting middleware.",
		"Add input validation for the user registration endpoint.",
		"Create a health check endpoint.",
		"Implement graceful shutdown.",
		"Add metrics collection for request latency.",
		"Refactor the configuration loading to use environment variables.",
	}

	return &RepositoryContextGenerator{
		numFiles:     numFiles,
		fileTokens:   fileTokens,
		files:        files,
		instructions: instructions,
	}
}

// Name returns the workload type name.
func (g *RepositoryContextGenerator) Name() string {
	return "repository_context"
}

// Generate produces a request with repository context and a code task.
func (g *RepositoryContextGenerator) Generate(iteration int) benchmark.WorkloadRequest {
	// Build a context message from the "repository files".
	var sb strings.Builder
	sb.WriteString("Here are the relevant files from the repository:\n\n")
	for i, f := range g.files {
		sb.WriteString(fmt.Sprintf("--- File: src/module_%d.go ---\n", i))
		sb.WriteString(f)
		sb.WriteString("\n--- End File ---\n\n")
	}

	instruction := g.instructions[iteration%len(g.instructions)]

	return benchmark.WorkloadRequest{
		SystemPrompt: "You are a senior software engineer helping with code review and improvements. " +
			"Analyze the provided repository files and follow the given instructions.",
		Messages: []benchmark.WorkloadMessage{
			{Role: "user", Content: sb.String()},
			{Role: "assistant", Content: "I've reviewed the repository files. What would you like me to do?"},
			{Role: "user", Content: instruction},
		},
	}
}

// generateRepoFile creates a synthetic Go source file.
func generateRepoFile(index, tokens int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package module%d\n\n", index))
	sb.WriteString("import (\n\t\"context\"\n\t\"fmt\"\n\t\"net/http\"\n)\n\n")
	sb.WriteString(fmt.Sprintf("// Handler%d processes requests for module %d.\n", index, index))
	sb.WriteString(fmt.Sprintf("type Handler%d struct {\n", index))
	sb.WriteString("\tdb     Database\n")
	sb.WriteString("\tlogger Logger\n")
	sb.WriteString("}\n\n")
	sb.WriteString(fmt.Sprintf("func NewHandler%d(db Database, logger Logger) *Handler%d {\n", index, index))
	sb.WriteString(fmt.Sprintf("\treturn &Handler%d{db: db, logger: logger}\n", index))
	sb.WriteString("}\n\n")
	sb.WriteString(fmt.Sprintf("func (h *Handler%d) ServeHTTP(w http.ResponseWriter, r *http.Request) {\n", index))
	sb.WriteString("\tctx := r.Context()\n")
	sb.WriteString(fmt.Sprintf("\tresult, err := h.process(ctx, r)\n"))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)\n")
	sb.WriteString("\t\treturn\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\tfmt.Fprint(w, result)\n")
	sb.WriteString("}\n\n")
	sb.WriteString(fmt.Sprintf("func (h *Handler%d) process(ctx context.Context, r *http.Request) (string, error) {\n", index))

	// Fill remaining tokens with function body.
	body := generateText(fmt.Sprintf("\t// Processing logic for module %d\n\t", index), tokens-100)
	sb.WriteString(body)
	sb.WriteString("\n\treturn \"ok\", nil\n}\n")

	return sb.String()
}
