package memory

import "container/list"

// lruTracker implements a least-recently-used eviction policy using a
// doubly-linked list and a map from cache key to list element. The most
// recently used entry is at the front; the least recently used is at the back.
//
// lruTracker is NOT thread-safe; callers must hold an external lock.
type lruTracker struct {
	order    *list.List
	elements map[string]*list.Element
}

// lruEntry is stored in each list element to map back to the cache key.
type lruEntry struct {
	key string
}

// newLRUTracker creates an empty LRU tracker.
func newLRUTracker() *lruTracker {
	return &lruTracker{
		order:    list.New(),
		elements: make(map[string]*list.Element),
	}
}

// Touch moves a key to the front (most recently used). If the key does
// not exist it is added at the front.
func (l *lruTracker) Touch(key string) {
	if elem, ok := l.elements[key]; ok {
		l.order.MoveToFront(elem)
		return
	}
	elem := l.order.PushFront(&lruEntry{key: key})
	l.elements[key] = elem
}

// Remove removes a key from the LRU tracker.
func (l *lruTracker) Remove(key string) {
	if elem, ok := l.elements[key]; ok {
		l.order.Remove(elem)
		delete(l.elements, key)
	}
}

// Oldest returns the key of the least recently used entry, or "" if the
// tracker is empty.
func (l *lruTracker) Oldest() string {
	back := l.order.Back()
	if back == nil {
		return ""
	}
	return back.Value.(*lruEntry).key
}

// Len returns the number of tracked entries.
func (l *lruTracker) Len() int {
	return l.order.Len()
}
