// Package blocks embeds the first-party registry: block files, fixtures and the registry version.
package blocks

import "embed"

// FS holds every first-party block, its fixtures and VERSION.
//
//go:embed *.py VERSION fixtures
var FS embed.FS
