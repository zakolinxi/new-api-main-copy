package common

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/requestpatch"

	"github.com/stretchr/testify/require"
)

func jsonPatchEffect(ops ...requestpatch.Operation) requestpatch.Effect {
	return requestpatch.Effect{Type: requestpatch.EffectJSONPatch, Ops: ops}
}

func headerPatchEffect(set map[string]string, remove []string) requestpatch.Effect {
	return requestpatch.Effect{Type: requestpatch.EffectHeaderPatch, Set: set, Remove: remove}
}

// A rewrite of the conversation or the model must be traceable; a rewrite of a
// sampling knob must not flood the consume log.
func TestRequestPatchAuditLinesFollowTheSensitivePathRule(t *testing.T) {
	effects := []requestpatch.Effect{
		jsonPatchEffect(
			requestpatch.Operation{Op: "replace", Path: "/messages/0/content"},
			requestpatch.Operation{Op: "replace", Path: "/model"},
			requestpatch.Operation{Op: "replace", Path: "/temperature"},
		),
	}

	lines := RequestPatchAuditLines(effects)

	require.Len(t, lines, 2, "only the messages and model rewrites are audited: %v", lines)
	// The trail uses the dotted form the param_override rule already speaks, so
	// an operator can compare the two without translating pointers by hand.
	require.Equal(t, []string{
		"request_patch: replace messages.0.content",
		"request_patch: replace model",
	}, lines)
}

// A pointer with no segments addresses the whole document: that always rewrites
// the prompts, so it is audited regardless of which operation it uses.
func TestRequestPatchAuditLinesAuditWholeDocumentRewrite(t *testing.T) {
	lines := RequestPatchAuditLines([]requestpatch.Effect{
		jsonPatchEffect(requestpatch.Operation{Op: "replace", Path: "/", Value: []byte(`{}`)}),
	})

	require.Equal(t, []string{"request_patch: replace (entire body)"}, lines)
}

func TestJSONPointerToAuditPathDecodesEscapes(t *testing.T) {
	require.Equal(t, "messages.0.content", jsonPointerToAuditPath("/messages/0/content"))
	require.Equal(t, "model", jsonPointerToAuditPath("/model"))
	require.Equal(t, "a/b", jsonPointerToAuditPath("/a~1b"))
	require.Equal(t, "a~b", jsonPointerToAuditPath("/a~0b"))
	require.Empty(t, jsonPointerToAuditPath(""))
	require.Empty(t, jsonPointerToAuditPath("/"))
}

func TestRequestPatchAuditLinesRecordHeaderChanges(t *testing.T) {
	effects := []requestpatch.Effect{
		headerPatchEffect(map[string]string{"X-Trace": "channel", "Authorization": "x"}, []string{"X-Old"}),
	}

	lines := RequestPatchAuditLines(effects)

	require.Equal(t, []string{
		"request_patch: set_header Authorization",
		"request_patch: set_header X-Trace",
		"request_patch: remove_header X-Old",
	}, lines, "header sets are sorted so two attempts produce the same trail")
}

// The trail records the operation and the path, never the value: a value can
// carry user content.
func TestRequestPatchAuditLinesOmitValues(t *testing.T) {
	effects := []requestpatch.Effect{
		jsonPatchEffect(requestpatch.Operation{
			Op:    "replace",
			Path:  "/messages/0/content",
			Value: []byte(`"secret user prompt"`),
		}),
	}

	lines := RequestPatchAuditLines(effects)

	require.Len(t, lines, 1)
	require.NotContains(t, lines[0], "secret user prompt")
}

func TestRequestPatchAuditLinesAreBounded(t *testing.T) {
	ops := make([]requestpatch.Operation, 0, requestPatchAuditLineLimit*2)
	for range requestPatchAuditLineLimit * 2 {
		ops = append(ops, requestpatch.Operation{Op: "remove", Path: "/messages/0"})
	}

	lines := RequestPatchAuditLines([]requestpatch.Effect{jsonPatchEffect(ops...)})

	require.Len(t, lines, requestPatchAuditLineLimit, "one attempt cannot flood the consume log")
}

func TestRequestPatchAuditLinesEmpty(t *testing.T) {
	require.Empty(t, RequestPatchAuditLines(nil))
}
