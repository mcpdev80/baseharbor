package runtime

import (
	"fmt"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/dcspki"
)

func ensureBootstrapEtcdTLS(stateDir string) error {
	_, err := dcspki.Ensure(
		filepath.Join(stateDir, "providers", "postgresql", "runtime", "etcd-pki"),
		[]string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"},
	)
	if err != nil {
		return fmt.Errorf("prepare PostgreSQL DCS mTLS: %w", err)
	}
	return nil
}
