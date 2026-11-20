package common

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/pkg/requestpatch"
)

// requestPatchAuditLineLimit bounds what one attempt can add to the consume
// log. A channel may declare up to 32 effects with 512 operations each, and the
// audit trail exists to trace a rewrite, not to archive it.
const requestPatchAuditLineLimit = 32

// jsonPointerToAuditPath turns an RFC 6902 pointer into the dotted form the
// existing sensitive-path rule uses: "/messages/0/content" becomes
// "messages.0.content". The two escaped characters of RFC 6901 (~1 for "/",
// ~0 for "~") are decoded first, so a pointer addressing a key that itself
// contains a slash still maps to the right name.
//
// An empty result means the pointer addresses the whole document.
func jsonPointerToAuditPath(pointer string) string {
	pointer = strings.TrimSpace(pointer)
	if pointer == "" || pointer == "/" {
		return ""
	}
	segments := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for index, segment := range segments {
		segment = strings.ReplaceAll(segment, "~1", "/")
		segment = strings.ReplaceAll(segment, "~0", "~")
		segments[index] = segment
	}
	return strings.Join(segments, ".")
}

// RequestPatchAuditLines renders the request patch effects that changed
// something worth auditing.
//
// Header changes are always reported: a header is how a credential or a routing
// hint reaches the upstream, so "the channel set X-Trace" is worth keeping.
// Body changes follow the same sensitive-path rule param_override uses, so
// rewriting messages, model or reasoning leaves a trace while rewriting
// temperature does not flood the log.
//
// Values are never recorded. An operation's value can carry user content, and
// the path plus the operation is what makes a rewrite traceable.
func RequestPatchAuditLines(effects []requestpatch.Effect) []string {
	if len(effects) == 0 {
		return nil
	}
	lines := make([]string, 0, len(effects))
	for _, effect := range effects {
		switch effect.Type {
		case requestpatch.EffectJSONPatch:
			for _, op := range effect.Ops {
				mode := strings.TrimSpace(op.Op)
				path := jsonPointerToAuditPath(op.Path)
				if path == "" {
					// Replacing the whole document always changes the prompts,
					// so it is audited no matter what the operation is called.
					lines = append(lines, "request_patch: "+mode+" (entire body)")
				} else if shouldAuditOperation(mode, path, jsonPointerToAuditPath(op.From), "") {
					lines = append(lines, fmt.Sprintf("request_patch: %s %s", mode, path))
				} else {
					continue
				}
				if len(lines) >= requestPatchAuditLineLimit {
					return lines
				}
			}
		case requestpatch.EffectHeaderPatch:
			names := make([]string, 0, len(effect.Set))
			for name := range effect.Set {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				lines = append(lines, "request_patch: set_header "+name)
			}
			for _, name := range effect.Remove {
				lines = append(lines, "request_patch: remove_header "+name)
			}
			if len(lines) >= requestPatchAuditLineLimit {
				return lines[:requestPatchAuditLineLimit]
			}
		}
	}
	return lines
}
