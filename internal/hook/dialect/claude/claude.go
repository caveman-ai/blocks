// Package claude is the dialect for Claude Code's hook JSON, which Codex CLI shares
// (docs/research/harness-hooks-2026-10-03.md).
package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/caveman-ai/blocks/internal/hook/protocol"
)

type input struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	ToolUseID     string `json:"tool_use_id"`
	HookEventName string `json:"hook_event_name"`
	ToolInput     struct {
		Command json.RawMessage `json:"command"` // a string, or an argv array on Codex
	} `json:"tool_input"`
}

type output struct {
	HookSpecificOutput *specific `json:"hookSpecificOutput,omitempty"`
}

type specific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// Parse reads a PreToolUse or PostToolUse payload.
func Parse(stdin []byte) (protocol.Request, error) {
	var in input
	if err := json.Unmarshal(stdin, &in); err != nil {
		return protocol.Request{}, fmt.Errorf("claude: %w", err)
	}
	r := protocol.Request{Protocol: protocol.Version, Session: in.SessionID, CallID: in.ToolUseID, Cwd: in.Cwd}
	switch in.HookEventName {
	case "PreToolUse":
		r.Phase = "pre"
	case "PostToolUse":
		r.Phase = "post"
	default:
		return protocol.Request{}, fmt.Errorf("claude: hook_event_name %q", in.HookEventName)
	}
	cmd, err := command(in.ToolInput.Command)
	if err != nil {
		return protocol.Request{}, err
	}
	r.Command = cmd
	return r, nil
}

// command accepts a string or an argv array, which it joins with shell quoting so rule 4 can unwrap
// `bash -lc '...'` the same way in both shapes.
func command(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var argv []string
	if err := json.Unmarshal(raw, &argv); err != nil {
		return "", fmt.Errorf("claude: tool_input.command is neither a string nor an argv array")
	}
	for i, a := range argv {
		argv[i] = quote(a)
	}
	return strings.Join(argv, " "), nil
}

func quote(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`;&|<>()*?[]{}~#!") {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// Render answers with additionalContext on the event that phase names. It never sets
// permissionDecision, so the harness's own permission flow is untouched. No hint renders `{}`.
func Render(resp protocol.Response, phase string) ([]byte, error) {
	if resp.Hint == "" {
		return json.Marshal(output{})
	}
	ev := "PreToolUse"
	if phase == "post" {
		ev = "PostToolUse"
	}
	return json.Marshal(output{&specific{HookEventName: ev, AdditionalContext: resp.Hint}})
}
