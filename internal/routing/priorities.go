package routing

import (
	"sort"
	"sync"
)

// ProviderPriority defines the priority of a provider for routing.
// Lower values indicate higher priority.
type ProviderPriority struct {
	// Provider is the provider name.
	Provider string `json:"provider"`
	// Priority is the priority value (lower = higher priority).
	Priority int `json:"priority"`
	// Weight is an optional weight for weighted random selection (0–100).
	Weight int `json:"weight,omitempty"`
}

// PriorityList maintains an ordered list of providers by priority.
// It is safe for concurrent use.
type PriorityList struct {
	mu         sync.RWMutex
	priorities []ProviderPriority
}

// NewPriorityList creates a PriorityList from the given priorities.
// The list is sorted by priority (ascending).
func NewPriorityList(priorities []ProviderPriority) *PriorityList {
	pl := &PriorityList{
		priorities: make([]ProviderPriority, len(priorities)),
	}
	copy(pl.priorities, priorities)
	pl.sortLocked()
	return pl
}

// Ordered returns the providers in priority order (highest priority first).
func (pl *PriorityList) Ordered() []string {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	result := make([]string, len(pl.priorities))
	for i, p := range pl.priorities {
		result[i] = p.Provider
	}
	return result
}

// Priorities returns a copy of all priorities.
func (pl *PriorityList) Priorities() []ProviderPriority {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	result := make([]ProviderPriority, len(pl.priorities))
	copy(result, pl.priorities)
	return result
}

// SetPriority sets the priority for a provider. If the provider is not
// in the list, it is added.
func (pl *PriorityList) SetPriority(provider string, priority int) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for i, p := range pl.priorities {
		if p.Provider == provider {
			pl.priorities[i].Priority = priority
			pl.sortLocked()
			return
		}
	}
	pl.priorities = append(pl.priorities, ProviderPriority{
		Provider: provider,
		Priority: priority,
	})
	pl.sortLocked()
}

// Remove removes a provider from the priority list.
func (pl *PriorityList) Remove(provider string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	for i, p := range pl.priorities {
		if p.Provider == provider {
			pl.priorities = append(pl.priorities[:i], pl.priorities[i+1:]...)
			return
		}
	}
}

// GetPriority returns the priority for a provider, or -1 if not found.
func (pl *PriorityList) GetPriority(provider string) int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	for _, p := range pl.priorities {
		if p.Provider == provider {
			return p.Priority
		}
	}
	return -1
}

// Size returns the number of providers in the list.
func (pl *PriorityList) Size() int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return len(pl.priorities)
}

// NextAfter returns the next provider in priority order after the given one.
// If the provider is not found or is last, it returns ("", false).
func (pl *PriorityList) NextAfter(provider string) (string, bool) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	for i, p := range pl.priorities {
		if p.Provider == provider && i+1 < len(pl.priorities) {
			return pl.priorities[i+1].Provider, true
		}
	}
	return "", false
}

// sortLocked sorts the priorities by priority value (ascending). Must be
// called with the write lock held.
func (pl *PriorityList) sortLocked() {
	sort.SliceStable(pl.priorities, func(i, j int) bool {
		return pl.priorities[i].Priority < pl.priorities[j].Priority
	})
}
