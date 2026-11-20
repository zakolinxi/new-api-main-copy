package model

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"golang.org/x/net/http/httpguts"
)

const (
	// maxHeaderOverrideValueBytes bounds one overridden header value. The value
	// is written verbatim onto the upstream request, and anything longer is
	// almost always a paste mistake rather than a deliberate setting.
	maxHeaderOverrideValueBytes = 8192
	// clientHeaderOverridePrefix is the placeholder that copies an incoming
	// client header onto the upstream request.
	clientHeaderOverridePrefix = "{client_header:"
)

// sensitivePassthroughHeaders are the client headers a channel must never copy
// onto its upstream request.
//
// The credentials in an incoming request belong to this gateway. Forwarding one
// to a provider hands that provider a working key for this deployment, and
// nothing legitimate needs it: a channel that wants its own credential writes
// {api_key}, which is substituted with the channel key.
var sensitivePassthroughHeaders = map[string]struct{}{
	"authorization":        {},
	"proxy-authorization":  {},
	"cookie":               {},
	"set-cookie":           {},
	"host":                 {},
	"x-api-key":            {},
	"x-goog-api-key":       {},
	"api-key":              {},
	"x-auth-token":         {},
	"x-amz-security-token": {},
}

// clientHeaderOverrideName reads a value that is exactly "{client_header:<name>}"
// and returns the header it copies.
//
// The runtime only substitutes a placeholder that is the whole value, so a value
// with surrounding text is an ordinary literal that happens to contain the
// prefix. This mirrors that rule so validation rejects exactly what the
// substitution would have used.
func clientHeaderOverrideName(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, clientHeaderOverridePrefix) {
		return "", false
	}
	rest := trimmed[len(clientHeaderOverridePrefix):]
	end := strings.Index(rest, "}")
	if end < 0 || end != len(rest)-1 {
		return "", false
	}
	name := strings.TrimSpace(rest[:end])
	if name == "" {
		return "", false
	}
	return name, true
}

// isHeaderOverrideRuleKey reports the two passthrough forms: "*" and a
// "re:"/"regex:" pattern. Their values are ignored by the runtime, so they carry
// no header name to validate.
func isHeaderOverrideRuleKey(name string) (isRegex bool, pattern string, ok bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false, "", false
	}
	if trimmed == "*" {
		return false, "", true
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"re:", "regex:"} {
		if strings.HasPrefix(lower, prefix) {
			return true, strings.TrimSpace(lower[len(prefix):]), true
		}
	}
	return false, "", false
}

// ValidateHeaderOverride checks a channel's header_override before it is saved.
//
// The runtime applies this value to every outbound request, so a malformed one
// only shows up as a failure on the next call through the channel. Checking it
// on write turns that into an immediate, explainable error:
//
//   - the value must be a JSON object of string values
//   - every key must be a valid header name, or one of the passthrough rules
//   - a value must not carry control characters (a header injection would
//     otherwise only fail inside net/http)
//   - a {client_header:...} placeholder must name a real header and must not
//     name a credential, which would leak the caller's key to the provider
//   - a passthrough pattern must compile, since the runtime rejects the whole
//     channel when it does not
func ValidateHeaderOverride(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Decode into any and assert the shape here instead of unmarshalling
	// straight into map[string]any: the codec in use decides whether a JSON
	// array or scalar is an error or a silently empty map, and this check must
	// not vary with that setting.
	var decoded any
	if err := common.UnmarshalJsonStr(raw, &decoded); err != nil {
		return fmt.Errorf("request header override must be a JSON object: %w", err)
	}
	if decoded == nil {
		return nil
	}
	declared, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("request header override must be a JSON object")
	}

	for key, value := range declared {
		name := strings.TrimSpace(key)
		if name == "" {
			return fmt.Errorf("request header override contains an empty header name")
		}
		if isRegex, pattern, isRule := isHeaderOverrideRuleKey(name); isRule {
			if isRegex {
				if pattern == "" {
					return fmt.Errorf("request header override passthrough pattern is empty")
				}
				if _, err := regexp.Compile(pattern); err != nil {
					return fmt.Errorf("request header override passthrough pattern %q is not a valid regular expression", pattern)
				}
			}
			continue
		}
		if !httpguts.ValidHeaderFieldName(name) {
			return fmt.Errorf("request header override header name %q is invalid", name)
		}

		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("request header override value for %q must be a string", name)
		}
		if strings.ContainsAny(text, "\r\n\x00") {
			return fmt.Errorf("request header override value for %q must not contain control characters", name)
		}
		if len(text) > maxHeaderOverrideValueBytes {
			return fmt.Errorf("request header override value for %q exceeds %d bytes", name, maxHeaderOverrideValueBytes)
		}

		source, ok := clientHeaderOverrideName(text)
		if !ok {
			continue
		}
		if !httpguts.ValidHeaderFieldName(source) {
			return fmt.Errorf("request header override copies %q, which is not a valid header name", source)
		}
		if _, sensitive := sensitivePassthroughHeaders[strings.ToLower(source)]; sensitive {
			return fmt.Errorf(
				"request header override must not copy the client header %q; use {api_key} for the channel credential",
				source,
			)
		}
	}
	return nil
}
