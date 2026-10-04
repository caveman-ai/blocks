// Package plugins embeds the harness plugin files that hooks install writes verbatim.
package plugins

import _ "embed"

// OpenCode is the OpenCode plugin; install bakes the binary path into its INSTALLED constant.
//
//go:embed caveman-blocks/opencode/caveman-blocks.js
var OpenCode string
