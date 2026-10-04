//go:build !unix

package capture

// oNoFollow is unavailable here; Windows needs a privilege to create symlinks.
// It duplicates repo.NoFollow on purpose: capture must not import repo.
const oNoFollow = 0
