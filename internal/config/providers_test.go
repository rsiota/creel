package config

import "testing"

func TestAddGetRemoveAIProvider(t *testing.T) {
	c := &Config{}

	if got := c.GetAIProvider("openai"); got != nil {
		t.Errorf("GetAIProvider on empty config = %v, want nil", got)
	}

	c.AddAIProvider(AIProvider{Name: "openai", APIKey: "sk-1"})
	c.AddAIProvider(AIProvider{Name: "anthropic", APIKey: "sk-2"})

	if got := c.GetAIProvider("openai"); got == nil || got.APIKey != "sk-1" {
		t.Errorf("GetAIProvider(openai) = %+v, want sk-1", got)
	}

	c.RemoveAIProvider("openai")
	if got := c.GetAIProvider("openai"); got != nil {
		t.Errorf("GetAIProvider after remove = %v, want nil", got)
	}
	if len(c.AI.Providers) != 1 || c.AI.Providers[0].Name != "anthropic" {
		t.Errorf("remaining providers = %+v, want [anthropic]", c.AI.Providers)
	}
}

// Removing a provider that is the configured default clears the default.
func TestRemoveAIProviderClearsDefault(t *testing.T) {
	c := &Config{AI: AIConfig{Default: "openai"}}
	c.AddAIProvider(AIProvider{Name: "openai"})
	c.AddAIProvider(AIProvider{Name: "anthropic"})

	c.RemoveAIProvider("openai")
	if c.AI.Default != "" {
		t.Errorf("Default = %q after removing the default provider, want empty", c.AI.Default)
	}
}

// Removing a non-default provider leaves the default untouched.
func TestRemoveAIProviderKeepsUnrelatedDefault(t *testing.T) {
	c := &Config{AI: AIConfig{Default: "openai"}}
	c.AddAIProvider(AIProvider{Name: "openai"})
	c.AddAIProvider(AIProvider{Name: "anthropic"})

	c.RemoveAIProvider("anthropic")
	if c.AI.Default != "openai" {
		t.Errorf("Default = %q, want openai", c.AI.Default)
	}
}

// Removing a missing provider is a no-op (does not panic or wipe the list).
func TestRemoveAIProviderMissing(t *testing.T) {
	c := &Config{}
	c.AddAIProvider(AIProvider{Name: "openai"})
	before := len(c.AI.Providers)
	c.RemoveAIProvider("nope")
	if len(c.AI.Providers) != before {
		t.Errorf("RemoveAIProvider(missing) changed length: %d -> %d", before, len(c.AI.Providers))
	}
}
