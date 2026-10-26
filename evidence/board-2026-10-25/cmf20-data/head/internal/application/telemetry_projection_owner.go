package application

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Observe the existing destination authority before delivery. This never
// repairs ownership or permissions, follows links or creates missing files.
func ValidateOTLPTLSProjection(files RuntimeFiles) error {
	if files.Bindings == "" || !filepath.IsAbs(files.Bindings) {
		return errors.New("OTLP TLS projection requires an absolute protected binding owner")
	}
	for _, dir := range []string{files.Bindings, filepath.Join(files.Bindings, "telemetry")} {
		owner, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			parent := filepath.Dir(dir)
			for {
				info, e := os.Lstat(parent)
				if e == nil {
					if !info.IsDir() || !ownedOTLPProjectionParent(info) || checkPrivateRuntimeOwnerParents(filepath.Join(parent, "projection-owner-check")) != nil {
						return errors.New("OTLP TLS projection has no protected existing parent")
					}
					break
				}
				if !errors.Is(e, os.ErrNotExist) || filepath.Dir(parent) == parent {
					return errors.New("OTLP TLS projection parent is unverifiable")
				}
				parent = filepath.Dir(parent)
			}
			continue
		}
		if err != nil || !owner.IsDir() || !bhruntime.ProtectedStateOwner(dir, owner) || checkPrivateRuntimeOwnerParents(filepath.Join(dir, "projection-owner-check")) != nil {
			return errors.New("OTLP TLS projection requires its original protected directory owner")
		}
	}
	for _, name := range []string{"ca.pem", "client-cert.pem", "client-key.pem"} {
		info, err := existingPrivateRuntimeOwner(filepath.Join(files.Bindings, "telemetry", name))
		if err != nil || info != nil && info.Mode().Perm()&0022 != 0 {
			return errors.New("OTLP TLS projection file owner is unverifiable")
		}
	}
	return nil
}

func ownedOTLPProjectionParent(info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.IsDir() && info.Mode().Perm()&0022 == 0 && owner.Uid == uint32(os.Geteuid())
}
