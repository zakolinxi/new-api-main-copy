package channel

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"

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

// cachedRequestPatch holds the decoded effects of one channel's setting.
//
// Decoding on every forwarded request would re-parse up to 32 effects and 512
// operations per call. The cache is keyed by channel id and invalidated by
// comparing the raw setting text, so an edited channel is picked up on the next
// request without restarting the node.
type cachedRequestPatch struct {
	raw     string
	effects []requestpatch.Effect
}

var requestPatchCache sync.Map // map[int]cachedRequestPatch

// channelRequestPatchEffects is the request-scoped form of
// ChannelRequestPatchEffects. A relayed request without channel metadata, or
// whose channel has no request_patch, yields no effects.
//
// The returned slice is shared with the cache and must be treated as read-only;
// the applier builds a filtered copy whenever it has to drop an effect.
func channelRequestPatchEffects(info *common.RelayInfo) ([]requestpatch.Effect, error) {
	if info == nil || info.ChannelMeta == nil {
		return nil, nil
	}
	setting := info.ChannelMeta.ChannelSetting
	raw := string(bytes.TrimSpace(setting.RequestPatch))
	if raw == "" || raw == "null" {
		return nil, nil
	}

	channelID := info.ChannelMeta.ChannelId
	if cached, ok := requestPatchCache.Load(channelID); ok {
		entry := cached.(cachedRequestPatch)
		if entry.raw == raw {
			return entry.effects, nil
		}
	}

	effects, err := ChannelRequestPatchEffects(setting)
	if err != nil {
		// A malformed setting is validated on save, so reaching this means the
		// row was edited directly. Do not cache it: the next write should be
		// observed without a restart.
		return nil, err
	}
	requestPatchCache.Store(channelID, cachedRequestPatch{raw: raw, effects: effects})
	return effects, nil
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
// and framing.
//
// applied reports the effects that took part, so the audit trail records what
// happened rather than what was configured, and bodyRewritten reports whether
// the body changed.
func applyRequestPatch(req *http.Request, body []byte, effects []requestpatch.Effect) (applied []requestpatch.Effect, bodyRewritten bool, err error) {
	applied = effects
	// A body-less request (GET, DELETE) or one that is not a JSON document has
	// nowhere to apply a json.patch. Keeping only the header effects is what
	// stops such a call from failing on a channel configured for JSON calls.
	if len(body) == 0 || !gjson.ValidBytes(body) {
		applied = headerOnlyEffects(effects)
		if len(applied) == 0 {
			return nil, false, nil
		}
	}
	patchedBody, patchedHeader, err := requestpatch.Apply(body, req.Header, applied, requestpatch.IsProtectedHeader)
	if err != nil {
		return nil, false, err
	}
	for _, effect := range applied {
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
	return applied, bodyRewritten, nil
}
