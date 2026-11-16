//go:build !windows

package runtime

import (
	"os"
	"syscall"
)

// ProtectedStateOwner verifies native authority for a private state file or
// directory. It observes metadata only; it never acquires or repairs ownership.
func ProtectedStateOwner(_ string, info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().Perm()&0077 == 0 && owner.Uid == uint32(os.Getuid()) && (info.IsDir() || owner.Nlink == 1)
}
