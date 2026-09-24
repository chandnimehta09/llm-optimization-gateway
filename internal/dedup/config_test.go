package dedup

import "testing"

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if !c.Enabled {
		t.Error("expected Enabled=true")
	}
	if c.Strategy != "exact" {
		t.Errorf("expected Strategy='exact', got %q", c.Strategy)
	}
	if !c.CaseSensitive {
		t.Error("expected CaseSensitive=true")
	}
	if c.MinContentLength != 0 {
		t.Errorf("expected MinContentLength=0, got %d", c.MinContentLength)
	}
	if c.ContentTypes != nil {
		t.Errorf("expected ContentTypes=nil, got %v", c.ContentTypes)
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	c := DefaultConfig()
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_EmptyStrategy(t *testing.T) {
	c := Config{Strategy: ""}
	if err := c.Validate(); err != nil {
		t.Errorf("empty strategy should be valid, got: %v", err)
	}
}

func TestConfig_Validate_InvalidStrategy(t *testing.T) {
	c := Config{Strategy: "fuzzy"}
	if err := c.Validate(); err != ErrUnknownStrategy {
		t.Errorf("expected ErrUnknownStrategy, got %v", err)
	}
}

func TestConfig_Validate_NegativeMinLength(t *testing.T) {
	c := Config{Strategy: "exact", MinContentLength: -1}
	if err := c.Validate(); err != ErrInvalidMinLength {
		t.Errorf("expected ErrInvalidMinLength, got %v", err)
	}
}
