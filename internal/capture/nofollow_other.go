//go:build !unix

package capture

// oNoFollow is unavailable here; Windows needs a privilege to create symlinks.
const oNoFollow = 0
