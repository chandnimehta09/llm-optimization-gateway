package fixtures

// RepositoryFixture represents a sample repository state for testing.
type RepositoryFixture struct {
	URL    string
	Branch string
	Commit string
}

// SampleRepositories contains sample repository states for testing.
var SampleRepositories = map[string]RepositoryFixture{
	"main_branch": {
		URL:    "https://github.com/example/project",
		Branch: "main",
		Commit: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
	},
	"feature_branch": {
		URL:    "https://github.com/example/project",
		Branch: "feature/new-api",
		Commit: "f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5",
	},
	"different_repo": {
		URL:    "https://github.com/other-org/other-project",
		Branch: "main",
		Commit: "1234567890abcdef1234567890abcdef12345678",
	},
	"same_repo_new_commit": {
		URL:    "https://github.com/example/project",
		Branch: "main",
		Commit: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	},
	"same_repo_diff_branch": {
		URL:    "https://github.com/example/project",
		Branch: "develop",
		Commit: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
	},
}

// FileHashFixture represents a set of file hashes for testing.
type FileHashFixture struct {
	Path string
	Hash string
}

// SampleFileHashes contains sample file hash sets for testing.
var SampleFileHashes = map[string][]FileHashFixture{
	"go_project": {
		{Path: "main.go", Hash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd"},
		{Path: "go.mod", Hash: "111222333444555666777888999000aaabbbcccdddeeefff000111222333444"},
		{Path: "internal/handler.go", Hash: "fedcba987654fedcba987654fedcba987654fedcba987654fedcba987654fedc"},
	},
	"go_project_modified": {
		{Path: "main.go", Hash: "modified_hash_abc123def456abc123def456abc123def456abc123def456abcd"},
		{Path: "go.mod", Hash: "111222333444555666777888999000aaabbbcccdddeeefff000111222333444"},
		{Path: "internal/handler.go", Hash: "fedcba987654fedcba987654fedcba987654fedcba987654fedcba987654fedc"},
	},
	"empty_project": {},
}

// DependencyFixture represents a dependency lock file for testing.
type DependencyFixture struct {
	Type string
	Hash string
}

// SampleDependencies contains sample dependency lock sets for testing.
var SampleDependencies = map[string][]DependencyFixture{
	"go_deps": {
		{Type: "go.sum", Hash: "gosum_hash_abc123"},
	},
	"node_deps": {
		{Type: "package-lock.json", Hash: "pkglock_hash_def456"},
		{Type: "yarn.lock", Hash: "yarnlock_hash_ghi789"},
	},
	"mixed_deps": {
		{Type: "go.sum", Hash: "gosum_hash_abc123"},
		{Type: "package-lock.json", Hash: "pkglock_hash_def456"},
	},
	"updated_go_deps": {
		{Type: "go.sum", Hash: "gosum_hash_updated_xyz789"},
	},
	"empty_deps": {},
}
