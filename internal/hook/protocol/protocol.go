// Package protocol is the generic hook JSON contract of docs/INTEGRATION.md. Every dialect translates
// a harness's JSON to and from it; `caveman-blocks hook --harness generic` speaks it directly.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Version is the protocol number this binary speaks.
const Version = 1

// Request is one hook call. Unknown fields are ignored on input.
type Request struct {
	Protocol int             `json:"protocol"`
	Phase    string          `json:"phase"` // pre | post
	Harness  string          `json:"harness"`
	Session  string          `json:"session"`
	CallID   string          `json:"call_id"`
	Cwd      string          `json:"cwd"`
	Command  string          `json:"command"`
	Output   json.RawMessage `json:"output,omitempty"` // post only, when the harness provides it
}

// Response is the answer. Action is always "allow" in v0; "deny" is reserved (decision 0010).
type Response struct {
	Protocol int    `json:"protocol"`
	Action   string `json:"action"`
	Reason   string `json:"reason"`
	Hint     string `json:"hint"`
}

// Allow is the v0 response carrying hint, which may be empty.
func Allow(hint string) Response {
	return Response{Protocol: Version, Action: "allow", Hint: hint}
}

// ParseRequest decodes and checks a generic request.
func ParseRequest(b []byte) (Request, error) {
	var r Request
	if err := json.Unmarshal(b, &r); err != nil {
		return Request{}, fmt.Errorf("protocol: %w", err)
	}
	if string(r.Output) == "null" {
		r.Output = nil
	}
	if r.Protocol != Version {
		return Request{}, fmt.Errorf("protocol: version %d, want %d", r.Protocol, Version)
	}
	if r.Phase != "pre" && r.Phase != "post" {
		return Request{}, fmt.Errorf("protocol: phase %q, want pre or post", r.Phase)
	}
	return r, nil
}
