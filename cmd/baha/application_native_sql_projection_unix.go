//go:build !windows

package main

import (
	"os"
	"syscall"
)

// Public native runtime projections are not private state. They must still be
// regular, owner-created, singly linked and not writable by other principals.
func sqlPublicProjectionOwner(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() &&
		stat.Uid == uint32(os.Getuid()) && stat.Nlink == 1 &&
		info.Mode().Perm()&0022 == 0 && info.Mode().Perm()&0044 == 0044
}
