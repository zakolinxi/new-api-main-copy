package channel

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
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

// readRequestBodyForPatch buffers the outbound body so a json.patch effect can
// rewrite it. Callers reach this only when the channel configured effects, so a
// channel without one keeps handing the original reader to net/http. The read is
// bounded by the same limit a client body is read with.
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
// is present; a header-only setting keeps the original reader and framing.
func applyRequestPatch(req *http.Request, body []byte, effects []requestpatch.Effect) error {
	patchedBody, patchedHeader, err := requestpatch.Apply(body, req.Header, effects, requestpatch.IsProtectedHeader)
	if err != nil {
		return err
	}
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
		break
	}
	req.Header = patchedHeader
	return nil
}
