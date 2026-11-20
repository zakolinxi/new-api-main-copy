package channel

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/tidwall/gjson"
)

// ChannelRequestPatchEffects decodes the channel's request_patch setting.
//
// The setting holds the effects array of the request patch contract: RFC 6902
// objects under a json.patch effect and set/remove header names under a
// header.patch effect. It is unset on every channel that was never configured
// with one, and then this returns no effects, which is the signal every caller
// uses to keep the untouced request path.
func ChannelRequestPatchEffects(setting dto.ChannelSettings) ([]requestpatch.Effect, error) {
	raw := bytes.TrimSpace(setting.RequestPatch)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	effects, err := requestpatch.DecodeEffects(raw)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("invalid channel request_patch setting: %w", err), types.ErrorCodeChannelRequestPatchInvalid)
	}
	return effects, nil
}

// channelRequestPatchEffects is the request-scoped form of
// ChannelRequestPatchEffects. A relayed request without channel metadata, or
// whose channel has no request_patch, yields no effects.
func channelRequestPatchEffects(info *common.RelayInfo) ([]requestpatch.Effect, error) {
	if info == nil || info.ChannelMeta == nil {
		return nil, nil
	}
	return ChannelRequestPatchEffects(info.ChannelMeta.ChannelSetting)
}

// isJSONRequestContentType reports whether a JsonPatch effect has a document to
// operate on. An empty Content-Type is treated as possibly JSON: many clients
// omit it, and applyRequestPatch still refuses to parse a body that turns out
// not to be JSON.
func isJSONRequestContentType(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	if mediaType == "application/json" || mediaType == "text/json" {
		return true
	}
	return strings.HasSuffix(mediaType, "+json")
}

// headerOnlyEffects drops JsonPatch effects and keeps the header ones, in order.
//
// A request whose body is absent or not JSON cannot carry a json.patch: the
// patch contract needs a JSON document to address. Dropping the effect instead
// of failing keeps a channel setting that targets JSON calls from breaking the
// bodyless and non-JSON calls that share the same channel.
func headerOnlyEffects(effects []requestpatch.Effect) []requestpatch.Effect {
	filtered := make([]requestpatch.Effect, 0, len(effects))
	for _, effect := range effects {
		if effect.Type == requestpatch.EffectHeaderPatch {
			filtered = append(filtered, effect)
		}
	}
	return filtered
}

// hasJSONPatchEffect reports whether any effect addresses the request body.
func hasJSONPatchEffect(effects []requestpatch.Effect) bool {
	for _, effect := range effects {
		if effect.Type == requestpatch.EffectJSONPatch {
			return true
		}
	}
	return false
}

// readRequestBodyForPatch buffers the outbound body so a json.patch effect can
// rewrite it. Callers reach this only when the channel configured a body effect
// and the request is JSON, so a channel without one keeps handing the original
// reader to net/http. The read is bounded by the same limit a client body is
// read with.
func readRequestBodyForPatch(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	limit := requestpatch.MaxBodyBytes()
	buffered, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read body for channel request patch failed: %w", err)
	}
	if int64(len(buffered)) > limit {
		return nil, fmt.Errorf("body exceeds the %d byte limit of a patched request", limit)
	}
	return buffered, nil
}

// applyRequestPatch rewrites the outgoing body and business headers in place.
//
// requestpatch.Apply is all or nothing: a rejected operation returns an error
// and leaves req untouched, so a bad channel setting can never send a half
// patched request upstream. The body is only replaced when a json.patch effect
// is actually applied; a header-only setting keeps the original reader
// and framing. Returns whether the body was rewritten.
func applyRequestPatch(req *http.Request, body []byte, effects []requestpatch.Effect) (bool, error) {
	// A body-less request (GET, DELETE) or one that is not a JSON document has
	// nowhere to apply a json.patch. Keeping only the header effects is what
	// stops such a call from failing on a channel configured for JSON calls.
	if len(body) == 0 || !gjson.ValidBytes(body) {
		effects = headerOnlyEffects(effects)
		if len(effects) == 0 {
			return false, nil
		}
	}
	patchedBody, patchedHeader, err := requestpatch.Apply(body, req.Header, effects, requestpatch.IsProtectedHeader)
	if err != nil {
		return false, err
	}
	bodyRewritten := false
	for _, effect := range effects {
		if effect.Type != requestpatch.EffectJSONPatch {
			continue
		}
		patched := bytes.Clone(patchedBody)
		req.Body = io.NopCloser(bytes.NewReader(patched))
		req.ContentLength = int64(len(patched))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(patched)), nil
		}
		bodyRewritten = true
		break
	}
	req.Header = patchedHeader
	return bodyRewritten, nil
}
