package setting

import (
	"strings"
	"testing"
)

func TestNormalizeTokenKeyPrefix(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: DefaultTokenKeyPrefix},
		{in: "   ", want: DefaultTokenKeyPrefix},
		{in: "sk-", want: "sk-"},
		{in: "sk-mao", want: "sk-mao-"},
		{in: "sk-mao-", want: "sk-mao-"},
		{in: "mao_2", want: "mao_2-"},
		{in: "sk mao", wantErr: true},
		{in: "sk/mao", wantErr: true},
		{in: strings.Repeat("a", MaxTokenKeyPrefixLength+1), wantErr: true},
	}
	for _, c := range cases {
		got, err := NormalizeTokenKeyPrefix(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("NormalizeTokenKeyPrefix(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeTokenKeyPrefix(%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("NormalizeTokenKeyPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimTokenKeyPrefixKeepsHistoricalKeysWorking(t *testing.T) {
	original := GetTokenKeyPrefix()
	t.Cleanup(func() { SetTokenKeyPrefix(original) })

	// Default prefix: the historical shape.
	SetTokenKeyPrefix(DefaultTokenKeyPrefix)
	if got := TrimTokenKeyPrefix("sk-abc123"); got != "abc123" {
		t.Fatalf("default prefix trim = %q, want abc123", got)
	}

	// A configured prefix must strip itself...
	SetTokenKeyPrefix("sk-mao-")
	if got := TrimTokenKeyPrefix("sk-mao-abc123"); got != "abc123" {
		t.Fatalf("custom prefix trim = %q, want abc123", got)
	}
	// ...and keys issued before the change must keep working, which is why the
	// fallback to "sk-" exists.
	if got := TrimTokenKeyPrefix("sk-abc123"); got != "abc123" {
		t.Fatalf("legacy key trim = %q, want abc123", got)
	}

	// A prefix without the sk- base works too.
	SetTokenKeyPrefix("mao-")
	if got := TrimTokenKeyPrefix("mao-abc123"); got != "abc123" {
		t.Fatalf("bare prefix trim = %q, want abc123", got)
	}
	if got := TrimTokenKeyPrefix("sk-abc123"); got != "abc123" {
		t.Fatalf("legacy key under bare prefix = %q, want abc123", got)
	}

	// An unprefixed key is returned untouched, which is how the internals store it.
	SetTokenKeyPrefix("sk-mao-")
	if got := TrimTokenKeyPrefix("abc123"); got != "abc123" {
		t.Fatalf("raw key trim = %q, want abc123", got)
	}
}

func TestTokenKeyPrefixExampleUsesConfiguredPrefix(t *testing.T) {
	original := GetTokenKeyPrefix()
	t.Cleanup(func() { SetTokenKeyPrefix(original) })

	SetTokenKeyPrefix("sk-mao-")
	if example := TokenKeyPrefixExample(); !strings.HasPrefix(example, "sk-mao-") {
		t.Fatalf("example %q does not start with the configured prefix", example)
	}
}
