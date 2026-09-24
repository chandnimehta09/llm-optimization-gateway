package fixtures

// ConversationFixture represents a conversation state for testing.
type ConversationFixture struct {
	TurnCount   int
	HistoryHash string
}

// SampleConversations contains sample conversation states for testing.
var SampleConversations = map[string]ConversationFixture{
	"empty": {
		TurnCount:   0,
		HistoryHash: "empty_conversation_hash",
	},
	"single_turn": {
		TurnCount:   1,
		HistoryHash: "single_turn_hash_abc123",
	},
	"multi_turn": {
		TurnCount:   5,
		HistoryHash: "multi_turn_hash_def456",
	},
	"long_conversation": {
		TurnCount:   50,
		HistoryHash: "long_conversation_hash_ghi789",
	},
	"modified_history": {
		TurnCount:   5,
		HistoryHash: "modified_multi_turn_hash_xyz999",
	},
}

// ConversationHistoryEntry represents a single conversation turn for testing.
type ConversationHistoryEntry struct {
	Role    string
	Content string
}

// SampleConversationHistories contains sample full conversation histories for testing.
var SampleConversationHistories = map[string][]ConversationHistoryEntry{
	"code_review_session": {
		{Role: "user", Content: "Can you review this Go function?"},
		{Role: "assistant", Content: "I'd be happy to review your Go function. Please share the code."},
		{Role: "user", Content: "func add(a, b int) int { return a + b }"},
		{Role: "assistant", Content: "The function looks correct. It takes two integers and returns their sum."},
	},
	"debugging_session": {
		{Role: "user", Content: "I'm getting a nil pointer dereference in my code."},
		{Role: "assistant", Content: "Can you share the stack trace and the relevant code?"},
		{Role: "user", Content: "Here's the error: panic: runtime error: nil pointer dereference"},
		{Role: "assistant", Content: "This typically happens when you try to access a field or method on a nil pointer."},
		{Role: "user", Content: "I found it - I forgot to check if the result was nil before accessing .Name"},
	},
	"empty_session": {},
}
