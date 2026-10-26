package etcd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// RecoveryClusterToken gives all members of a restored snapshot the same new
// cluster identity, distinct from either live bootstrap token. It is stable
// on resume and bound to the owned snapshot, not to a guessed directory.
func RecoveryClusterToken(ctx context.Context, archive string, identity Identity) (string, error) {
	if err := identity.Validate(); err != nil {
		return "", err
	}
	if err := safeRegular(archive); err != nil {
		return "", err
	}
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	for _, field := range []string{identity.Core, identity.Target, identity.Cluster} {
		_, _ = io.WriteString(h, field+"\x00")
	}
	if _, err := io.Copy(h, &contextReader{ctx: ctx, Reader: f}); err != nil {
		return "", err
	}
	return "baseharbor-recovery-" + hex.EncodeToString(h.Sum(nil)), nil
}
