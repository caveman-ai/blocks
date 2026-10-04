//go:build !windows

package repo

import "syscall"

// NoFollow makes OpenFile refuse a symlink as the final path element.
const NoFollow = syscall.O_NOFOLLOW
