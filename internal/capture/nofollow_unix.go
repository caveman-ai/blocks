//go:build unix

package capture

import "syscall"

// oNoFollow makes opening a symlink fail instead of following it.
const oNoFollow = syscall.O_NOFOLLOW
