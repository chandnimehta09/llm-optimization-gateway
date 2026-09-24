package exact

import (
	"strconv"

	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/request"
)

// DeduplicateMessages removes duplicate text content from normalized messages.
// It extracts text content from messages, deduplicates by value, and returns
// the filtered messages along with dedup statistics. Messages with tool calls
// or tool call IDs are always preserved.
func DeduplicateMessages(messages []request.Message, config dedup.Config) ([]request.Message, *dedup.DedupStats) {
	if len(messages) == 0 {
		stats := &dedup.DedupStats{
			DuplicateCounts: map[string]int{},
			Strategy:        "exact",
		}
		return messages, stats
	}

	// Extract content items from messages.
	items := extractContentItems(messages)

	// Deduplicate the extracted items.
	result := deduplicateItems(items, config)
	result.Stats.Strategy = "exact"

	// Rebuild the message list based on kept items.
	keptMessages := rebuildMessages(messages, result)

	return keptMessages, &result.Stats
}

// extractContentItems converts messages into ContentItems for deduplication.
// Each message becomes a content item with its text value extracted.
func extractContentItems(messages []request.Message) []dedup.ContentItem {
	items := make([]dedup.ContentItem, 0, len(messages))
	for i, msg := range messages {
		ct := messageContentType(msg)
		value := messageTextContent(msg)
		items = append(items, dedup.ContentItem{
			ID:    msg.ToolCallID,
			Type:  ct,
			Value: value,
			Metadata: map[string]string{
				"index": strconv.Itoa(i),
				"role":  msg.Role,
			},
		})
	}
	return items
}

// messageContentType maps a message role to a ContentType.
func messageContentType(msg request.Message) dedup.ContentType {
	switch msg.Role {
	case request.RoleSystem:
		return dedup.ContentTypeInstruction
	case request.RoleTool:
		return dedup.ContentTypeToolOutput
	default:
		return dedup.ContentTypeDocument
	}
}

// messageTextContent extracts the primary text content of a message.
func messageTextContent(msg request.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	// For multi-part messages, concatenate text parts.
	var text string
	for _, p := range msg.Parts {
		if p.Type == "text" {
			if text != "" {
				text += "\n"
			}
			text += p.Text
		}
	}
	return text
}

// rebuildMessages creates a new message slice preserving only non-duplicate messages.
// Messages with tool calls or tool call IDs are always kept regardless of dedup results.
func rebuildMessages(original []request.Message, result *dedup.DeduplicatedContent) []request.Message {
	removedSet := make(map[int]struct{}, len(result.RemovedDuplicates))
	for _, dup := range result.RemovedDuplicates {
		removedSet[dup.RemovedIndex] = struct{}{}
	}

	kept := make([]request.Message, 0, len(original))
	for i, msg := range original {
		// Always keep messages with tool calls — they carry function invocations.
		if len(msg.ToolCalls) > 0 {
			kept = append(kept, msg)
			continue
		}
		// Always keep tool response messages.
		if msg.ToolCallID != "" {
			kept = append(kept, msg)
			continue
		}
		if _, removed := removedSet[i]; !removed {
			kept = append(kept, msg)
		}
	}
	return kept
}
