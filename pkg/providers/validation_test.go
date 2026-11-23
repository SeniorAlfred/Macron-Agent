package providers

import (
	"strings"
	"testing"

	"github.com/SeniorAlfred/macron-agent/pkg/llm"
	"github.com/SeniorAlfred/macron-agent/pkg/store"
)

func TestValidateBaseURL(t *testing.T) {
	for _, ok := range []string{"https://api.openai.com/v1", "http://127.0.0.1:11434/v1", "https://host"} {
		if err := ValidateBaseURL(ok); err != nil {
			t.Fatalf("%q should be accepted: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "api.example.com/v1", "ftp://host/v1", "https://user:pass@host/v1", "https://host/v1?x=1", "https://host/v1#frag"} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

func TestNormalizeTrimsAndChecksTheImportantFields(t *testing.T) {
	p, err := Normalize(store.Provider{Name: "  My Provider ", BaseURL: "https://api.example.com/v1", Model: " gpt-5 ", APIKey: " sk-1 "})
	if err != nil {
		t.Fatalf("a normal provider should pass: %v", err)
	}
	if p.Name != "My Provider" || p.Model != "gpt-5" || p.APIKey != "sk-1" {
		t.Fatalf("the fields were not trimmed: %+v", p)
	}

	cases := []struct {
		name string
		p    store.Provider
		want string
	}{
		{"no name", store.Provider{BaseURL: "https://h/v1", Model: "m"}, "name"},
		{"no model", store.Provider{Name: "n", BaseURL: "https://h/v1"}, "model"},
		{"newline in key", store.Provider{Name: "n", BaseURL: "https://h/v1", Model: "m", APIKey: "a\nb"}, "single line"},
		{"unknown api", store.Provider{Name: "n", BaseURL: "https://h/v1", Model: "m", API: "soap"}, "unsupported API"},
		{"tiny window", store.Provider{Name: "n", BaseURL: "https://h/v1", Model: "m", ContextWindow: 100}, "context window"},
		{"negative priority", store.Provider{Name: "n", BaseURL: "https://h/v1", Model: "m", Priority: -1}, "priority"},
		{"bad effort", store.Provider{Name: "n", BaseURL: "https://h/v1", Model: "m", Effort: "turbo"}, "effort"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Normalize(c.p)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error about %q, got %v", c.want, err)
			}
		})
	}
}

func TestAPIFollowsTheExplicitTypeThenTheAddress(t *testing.T) {
	if got := API(store.Provider{API: llm.APIAnthropic, BaseURL: "https://api.openai.com/v1"}); got != llm.APIAnthropic {
		t.Fatalf("an explicit API type must win, got %q", got)
	}
	if got := API(store.Provider{BaseURL: "https://api.anthropic.com/v1"}); got != llm.APIAnthropic {
		t.Fatalf("an Anthropic address should be detected, got %q", got)
	}
	if got := API(store.Provider{BaseURL: "https://api.openai.com/v1"}); got != llm.APIOpenAI {
		t.Fatalf("an OpenAI-compatible address should stay OpenAI, got %q", got)
	}
}

func TestTemperatureLimitsFollowTheAPI(t *testing.T) {
	hot := 1.8
	// OpenAI-compatible endpoints accept up to 2, Anthropic only up to 1.
	if _, err := Normalize(store.Provider{Name: "n", BaseURL: "https://api.openai.com/v1", Model: "m", Temperature: &hot}); err != nil {
		t.Fatalf("1.8 is fine for OpenAI: %v", err)
	}
	if _, err := Normalize(store.Provider{Name: "n", BaseURL: "https://api.anthropic.com/v1", Model: "m", Temperature: &hot}); err == nil {
		t.Fatal("1.8 must be refused for Anthropic")
	}
}
