package channel

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/stretchr/testify/require"
)

func TestChannelRequestPatchEffectsUnset(t *testing.T) {
	effects, err := ChannelRequestPatchEffects(dto.ChannelSettings{})
	require.NoError(t, err)
	require.Empty(t, effects, "an unconfigured channel must not patch anything")

	for _, raw := range []string{"", "   ", "null"} {
		effects, err = ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(raw)})
		require.NoError(t, err, "value %q", raw)
		require.Empty(t, effects, "value %q", raw)
	}
}

func TestChannelRequestPatchEffectsRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"not an array":        `{"type":"json.patch"}`,
		"unknown effect type": `[{"type":"path.rewrite"}]`,
		"unknown member":      `[{"type":"json.patch","ops":[],"extra":true}]`,
		"bad operation":       `[{"type":"json.patch","ops":[{"op":"merge","path":"/model"}]}]`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(raw)})
			require.Error(t, err)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, types.ErrorCodeChannelRequestPatchInvalid, apiErr.GetErrorCode())
		})
	}
}

func TestChannelRequestPatchEffectsDecodesConfiguredValue(t *testing.T) {
	setting := dto.ChannelSettings{RequestPatch: json.RawMessage(
		`[{"type":"json.patch","ops":[{"op":"replace","path":"/temperature","value":0.2}]}]`)}
	effects, err := ChannelRequestPatchEffects(setting)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	require.Equal(t, "json.patch", effects[0].Type)
	require.Len(t, effects[0].Ops, 1)
}

func TestApplyRequestPatchRewritesBodyAndHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "https://upstream.test/v1/chat/completions", strings.NewReader(`{"model":"m","temperature":0.9}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Old", "keep")

	effects, err := ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(`[
		{"type":"json.patch","ops":[{"op":"replace","path":"/temperature","value":0.1}]},
		{"type":"header.patch","set":{"X-Trace":"channel"},"remove":["X-Old"]}
	]`)})
	require.NoError(t, err)

	rewritten, err := applyRequestPatch(req, []byte(`{"model":"m","temperature":0.9}`), effects)
	require.NoError(t, err)
	require.True(t, rewritten, "a json.patch effect must report the body it rewrote")

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"m","temperature":0.1}`, string(body))
	require.Equal(t, int64(len(body)), req.ContentLength, "framing must follow the patched body")

	replayed, err := req.GetBody()
	require.NoError(t, err, "GetBody must replay the patched body for retries")
	replayedBody, err := io.ReadAll(replayed)
	require.NoError(t, err)
	require.Equal(t, string(body), string(replayedBody))

	require.Equal(t, "channel", req.Header.Get("X-Trace"))
	require.Empty(t, req.Header.Get("X-Old"))
	require.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestApplyRequestPatchRejectsProtectedHeader(t *testing.T) {
	body := []byte(`{"model":"m"}`)
	req := httptest.NewRequest("POST", "https://upstream.test/v1/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer channel-key")

	effects, err := ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(
		`[{"type":"header.patch","set":{"Authorization":"Bearer stolen"}}]`)})
	require.NoError(t, err)

	_, err = applyRequestPatch(req, body, effects)
	require.Error(t, err, "credentials and framing headers stay protected")
	require.Contains(t, err.Error(), "protected")
}

func TestApplyRequestPatchLeavesFramingOfHeaderOnlyPatch(t *testing.T) {
	req := httptest.NewRequest("GET", "https://upstream.test/v1/models", nil)
	require.Equal(t, int64(0), req.ContentLength)
	originalBody := req.Body

	effects, err := ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(
		`[{"type":"header.patch","set":{"X-Trace":"channel"}}]`)})
	require.NoError(t, err)

	rewritten, err := applyRequestPatch(req, nil, effects)
	require.NoError(t, err)
	require.False(t, rewritten)
	require.Equal(t, originalBody, req.Body, "a header-only patch must not replace the body")
	require.Equal(t, int64(0), req.ContentLength)
	require.Equal(t, "channel", req.Header.Get("X-Trace"))
}

// A channel configured with a json.patch is reached by bodyless and non-JSON
// calls too (model lists, form uploads). Those must keep working with the
// header effects applied, instead of failing on a document that does not exist.
func TestApplyRequestPatchSkipsJSONPatchWithoutJSONBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "no body", body: nil},
		{name: "empty body", body: []byte{}},
		{name: "form body", body: []byte("grant_type=client_credentials&scope=read")},
		{name: "truncated json", body: []byte(`{"model":`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://upstream.test/v1/chat/completions", nil)

			effects, err := ChannelRequestPatchEffects(dto.ChannelSettings{RequestPatch: json.RawMessage(`[
				{"type":"json.patch","ops":[{"op":"replace","path":"/temperature","value":0.1}]},
				{"type":"header.patch","set":{"X-Trace":"channel"}}
			]`)})
			require.NoError(t, err)

			rewritten, err := applyRequestPatch(req, tt.body, effects)
			require.NoError(t, err, "a missing JSON document must not fail the request")
			require.False(t, rewritten)
			require.Equal(t, "channel", req.Header.Get("X-Trace"), "header effects still apply")
		})
	}
}

func TestIsJSONRequestContentType(t *testing.T) {
	for _, value := range []string{
		"",
		"application/json",
		"application/json; charset=utf-8",
		"text/json",
		"application/vnd.api+json",
	} {
		require.True(t, isJSONRequestContentType(value), "value %q", value)
	}
	for _, value := range []string{
		"multipart/form-data; boundary=x",
		"application/x-www-form-urlencoded",
		"application/octet-stream",
		"not a media type",
	} {
		require.False(t, isJSONRequestContentType(value), "value %q", value)
	}
}

func TestChannelRequestPatchEffectsIgnoresChannelsWithoutMeta(t *testing.T) {
	effects, err := channelRequestPatchEffects(nil)
	require.NoError(t, err)
	require.Empty(t, effects)

	effects, err = channelRequestPatchEffects(&common.RelayInfo{})
	require.NoError(t, err)
	require.Empty(t, effects)
}
