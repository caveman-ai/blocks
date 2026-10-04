// Package generic is the identity dialect: the harness speaks the generic protocol itself.
package generic

import (
	"encoding/json"

	"github.com/caveman-ai/blocks/internal/hook/protocol"
)

// Parse decodes a generic protocol request.
func Parse(stdin []byte) (protocol.Request, error) { return protocol.ParseRequest(stdin) }

// Render encodes resp unchanged; the phase does not alter the generic shape.
func Render(resp protocol.Response, phase string) ([]byte, error) { return json.Marshal(resp) }
