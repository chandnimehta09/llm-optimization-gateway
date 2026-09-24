package fixtures

// ToolSchemaFixture represents a tool schema for testing.
type ToolSchemaFixture struct {
	Name       string
	Version    string
	SchemaHash string
}

// SampleToolSchemas contains sample tool schema sets for testing.
var SampleToolSchemas = map[string][]ToolSchemaFixture{
	"search_v1": {
		{Name: "web_search", Version: "v1.0", SchemaHash: "search_schema_hash_v1"},
	},
	"search_v2": {
		{Name: "web_search", Version: "v2.0", SchemaHash: "search_schema_hash_v2"},
	},
	"multi_tool": {
		{Name: "web_search", Version: "v1.0", SchemaHash: "search_schema_hash_v1"},
		{Name: "calculator", Version: "v1.0", SchemaHash: "calc_schema_hash_v1"},
		{Name: "file_reader", Version: "v1.0", SchemaHash: "reader_schema_hash_v1"},
	},
	"multi_tool_updated": {
		{Name: "web_search", Version: "v1.0", SchemaHash: "search_schema_hash_v1"},
		{Name: "calculator", Version: "v2.0", SchemaHash: "calc_schema_hash_v2"},
		{Name: "file_reader", Version: "v1.0", SchemaHash: "reader_schema_hash_v1"},
	},
	"empty_tools": {},
}

// ToolOutputFixture represents a tool output for testing.
type ToolOutputFixture struct {
	ToolName   string
	OutputHash string
}

// SampleToolOutputs contains sample tool output sets for testing.
var SampleToolOutputs = map[string][]ToolOutputFixture{
	"search_result": {
		{ToolName: "web_search", OutputHash: "search_output_hash_abc"},
	},
	"multi_output": {
		{ToolName: "web_search", OutputHash: "search_output_hash_abc"},
		{ToolName: "calculator", OutputHash: "calc_output_hash_def"},
	},
	"updated_search_result": {
		{ToolName: "web_search", OutputHash: "search_output_hash_xyz_updated"},
	},
	"empty_outputs": {},
}
