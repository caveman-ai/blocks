// Package cursor is the dialect for Cursor's native hooks: beforeShellExecution (no context field) and
// afterShellExecution (additional_context), docs/research/harness-hooks-2026-10-03.md.
package cursor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/JuliusBrussee/caveman-blocks/internal/hook/protocol"
)

type input struct {
	ConversationID string   `json:"conversation_id"`
	GenerationID   string   `json:"generation_id"`
	HookEventName  string   `json:"hook_event_name"`
	Command        string   `json:"command"`
	Cwd            string   `json:"cwd"`
	WorkspaceRoots []string `json:"workspace_roots"`
}

// preOutput is intentionally empty: the hook must never widen permissions, and an explicit
// "allow" could skip Cursor's own command approval. Cursor treats a valid JSON object with no
// permission field as "no opinion"; confirmed during phase 1 dogfood.
type preOutput struct{}

type postOutput struct {
	AdditionalContext string `json:"additional_context,omitempty"`
}

// Parse reads a beforeShellExecution or afterShellExecution payload. Phase is "" when
// hook_event_name is absent; the caller then sets it from `--phase`.
//
// afterShellExecution carries no cwd, so Cwd falls back to the first workspace root, and CallID is
// derived from generation_id and the command, which both events carry, so the post-run replay finds
// the pre-run decision. generation_id alone spans a whole agent turn and is not a per-call id.
func Parse(stdin []byte) (protocol.Request, error) {
	var in input
	if err := json.Unmarshal(stdin, &in); err != nil {
		return protocol.Request{}, fmt.Errorf("cursor: %w", err)
	}
	r := protocol.Request{Protocol: protocol.Version, Session: in.ConversationID, Cwd: in.Cwd, Command: in.Command}
	switch in.HookEventName {
	case "beforeShellExecution":
		r.Phase = "pre"
	case "afterShellExecution":
		r.Phase = "post"
	case "":
	default:
		return protocol.Request{}, fmt.Errorf("cursor: hook_event_name %q", in.HookEventName)
	}
	if r.Cwd == "" && len(in.WorkspaceRoots) > 0 {
		r.Cwd = in.WorkspaceRoots[0]
	}
	if in.GenerationID != "" {
		h := sha256.Sum256([]byte(in.Command))
		r.CallID = in.GenerationID + ":" + hex.EncodeToString(h[:8])
	}
	return r, nil
}

// Render answers beforeShellExecution with an empty object, since it has no context field and we
// never decide permissions, and afterShellExecution with the hint as additional_context.
func Render(resp protocol.Response, phase string) ([]byte, error) {
	if phase == "post" {
		return json.Marshal(postOutput{AdditionalContext: resp.Hint})
	}
	return json.Marshal(preOutput{})
}
