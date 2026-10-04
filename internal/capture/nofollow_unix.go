//go:build unix

package capture

import "syscall"

// oNoFollow makes opening a symlink fail instead of following it.
// It duplicates repo.NoFollow on purpose: capture must not import repo.
const oNoFollow = syscall.O_NOFOLLOW
