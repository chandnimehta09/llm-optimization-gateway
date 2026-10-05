package scenarios

import "fmt"

// Shared error constructors for consistent error messages across scenarios.

func errMissingProvider(name string) error {
	return fmt.Errorf("test environment missing %q provider", name)
}

func errMissingComponent(name string) error {
	return fmt.Errorf("test environment missing component: %s", name)
}

func errUnexpectedStatus(expected, got int) error {
	return fmt.Errorf("expected status %d, got %d", expected, got)
}

func errInvalidJSON(err error) error {
	return fmt.Errorf("invalid JSON response: %w", err)
}

func errMissingField(field string) error {
	return fmt.Errorf("response missing required field: %s", field)
}

func errEmptyChoices() error {
	return fmt.Errorf("response has empty or invalid choices array")
}

func errInvalidChoiceFormat() error {
	return fmt.Errorf("response choice has invalid format")
}

func errEmptyContent() error {
	return fmt.Errorf("response content is empty")
}

func errUnexpectedValue(field, expected, got string) error {
	return fmt.Errorf("field %q: expected %q, got %q", field, expected, got)
}

func errCacheOperation(op string, err error) error {
	return fmt.Errorf("cache %s failed: %w", op, err)
}

func errCacheMiss(key string) error {
	return fmt.Errorf("expected cache hit for key %q but got miss", key)
}

func errUnexpectedCacheHit(key string) error {
	return fmt.Errorf("expected cache miss for key %q but got hit", key)
}

func errDeduplicationFailed(actual, expected int) error {
	return fmt.Errorf("deduplication failed: expected %d provider call(s), got %d", expected, actual)
}

func errDeduplicationIncomplete(actual, expected int) error {
	return fmt.Errorf("deduplication incomplete: %d of %d requests received response", actual, expected)
}

func errRoutingCollision(id string) error {
	return fmt.Errorf("routing collision: both providers returned same id %q", id)
}
