package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHeaderOverrideAcceptsRuntimeForms(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "whitespace only", raw: "   \n\t "},
		{name: "json null", raw: "null"},
		{name: "empty object", raw: "{}"},
		{
			name: "passthrough all",
			raw:  `{"*": ""}`,
		},
		{
			name: "passthrough regex",
			raw:  `{"re:^X-Trace-.*$": true, "regex:^X-Req-": 1}`,
		},
		{
			// The channel credential belongs to this deployment, so an explicit
			// Authorization override is a normal configuration, not a leak.
			name: "explicit credential header with channel key",
			raw:  `{"Authorization": "Bearer {api_key}"}`,
		},
		{
			name: "client header placeholder for a non-sensitive header",
			raw:  `{"X-Upstream-Trace": "{client_header:X-Trace-Id}"}`,
		},
		{
			name: "placeholder with padded name",
			raw:  `{"X-Upstream-Trace": "  {client_header: X-Trace-Id}  "}`,
		},
		{
			// Only a value that is exactly the placeholder is substituted, so
			// this is an ordinary literal string.
			name: "placeholder embedded in text",
			raw:  `{"X-Note": "from {client_header:Authorization} please"}`,
		},
		{
			name: "channel test template from the console",
			raw: `{
				"*": true,
				"re:^X-Trace-.*$": true,
				"X-Foo": "{client_header:X-Foo}",
				"Authorization": "Bearer {api_key}",
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"
			}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestValidateHeaderOverrideRejectsNonObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "array", raw: `["X-Foo"]`},
		{name: "string", raw: `"X-Foo"`},
		{name: "number", raw: `42`},
		{name: "boolean", raw: `true`},
		{name: "malformed", raw: `{"X-Foo":`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestValidateHeaderOverrideRejectsInvalidHeaderNames(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "space inside", raw: `{"Bad Header": "value"}`},
		{name: "colon inside", raw: `{"X-Foo:Bar": "value"}`},
		{name: "empty key", raw: `{"": "value"}`},
		{name: "blank key", raw: `{"   ": "value"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestValidateHeaderOverrideRejectsNonStringValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "number", raw: `{"X-Foo": 1.5}`},
		{name: "boolean", raw: `{"X-Foo": true}`},
		{name: "null", raw: `{"X-Foo": null}`},
		{name: "object", raw: `{"X-Foo": {"a": "b"}}`},
		{name: "array", raw: `{"X-Foo": ["a"]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestValidateHeaderOverrideRejectsHeaderInjection(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "carriage return", raw: `{"X-Foo": "value\rX-Injected: 1"}`},
		{name: "line feed", raw: `{"X-Foo": "value\nX-Injected: 1"}`},
		{name: "nul byte", raw: `{"X-Foo": "value\u0000tail"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateHeaderOverride(test.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), "control characters")
		})
	}
}

func TestValidateHeaderOverrideRejectsOversizedValue(t *testing.T) {
	oversized := strings.Repeat("a", maxHeaderOverrideValueBytes+1)
	require.Error(t, ValidateHeaderOverride(`{"X-Foo": "`+oversized+`"}`))

	// The bound is inclusive: exactly the limit is still accepted.
	atLimit := strings.Repeat("a", maxHeaderOverrideValueBytes)
	require.NoError(t, ValidateHeaderOverride(`{"X-Foo": "`+atLimit+`"}`))
}

func TestValidateHeaderOverrideRejectsCredentialPassthrough(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "authorization", source: "Authorization"},
		{name: "authorization lower", source: "authorization"},
		{name: "proxy authorization", source: "Proxy-Authorization"},
		{name: "cookie", source: "Cookie"},
		{name: "host", source: "Host"},
		{name: "openai key", source: "X-Api-Key"},
		{name: "google key", source: "X-Goog-Api-Key"},
		{name: "generic api key", source: "Api-Key"},
		{name: "auth token", source: "X-Auth-Token"},
		{name: "aws session token", source: "X-Amz-Security-Token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := `{"X-Leak": "{client_header:` + test.source + `}"}`
			err := ValidateHeaderOverride(raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), "use {api_key}")
		})
	}
}

func TestValidateHeaderOverrideRejectsBadPlaceholderName(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "space in name", raw: `{"X-Leak": "{client_header:Not A Header}"}`},
		{name: "colon in name", raw: `{"X-Leak": "{client_header:a:b}"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestValidateHeaderOverrideRejectsBadPassthroughPattern(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unclosed group", raw: `{"re:^(X-": true}`},
		{name: "bad repetition", raw: `{"regex:*abc": true}`},
		{name: "empty pattern", raw: `{"re:": true}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, ValidateHeaderOverride(test.raw))
		})
	}
}

func TestHeaderPassthroughRuleKeyRecognition(t *testing.T) {
	tests := []struct {
		key     string
		isRule  bool
		isRegex bool
		pattern string
	}{
		{key: "*", isRule: true, isRegex: false},
		{key: " * ", isRule: true, isRegex: false},
		{key: "re:^X-", isRule: true, isRegex: true, pattern: "^x-"},
		{key: "RE:^X-", isRule: true, isRegex: true, pattern: "^x-"},
		{key: "regex:^X-", isRule: true, isRegex: true, pattern: "^x-"},
		{key: "X-Foo", isRule: false},
		{key: "", isRule: false},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			isRegex, pattern, ok := isHeaderOverrideRuleKey(test.key)
			require.Equal(t, test.isRule, ok)
			require.Equal(t, test.isRegex, isRegex)
			if test.isRule && test.isRegex {
				require.Equal(t, test.pattern, pattern)
			}
		})
	}
}

func TestClientHeaderOverrideName(t *testing.T) {
	tests := []struct {
		value string
		name  string
		ok    bool
	}{
		{value: "{client_header:X-Trace}", name: "X-Trace", ok: true},
		{value: "  {client_header: X-Trace}  ", name: "X-Trace", ok: true},
		{value: "{client_header:}", ok: false},
		{value: "{client_header: }", ok: false},
		{value: "{client_header:X-Trace", ok: false},
		{value: "{client_header:X-Trace}suffix", ok: false},
		{value: "prefix{client_header:X-Trace}", ok: false},
		{value: "Bearer {api_key}", ok: false},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			name, ok := clientHeaderOverrideName(test.value)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.name, name)
		})
	}
}

func TestValidateSettingsRejectsUnsafeHeaderOverride(t *testing.T) {
	unsafe := `{"X-Leak": "{client_header:Authorization}"}`
	channel := &Channel{HeaderOverride: &unsafe}
	require.ErrorContains(t, channel.ValidateSettings(), "use {api_key}")

	safe := `{"Authorization": "Bearer {api_key}"}`
	channel = &Channel{HeaderOverride: &safe}
	require.NoError(t, channel.ValidateSettings())
}
