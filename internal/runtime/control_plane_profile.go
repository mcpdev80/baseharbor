package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A topology is explicit persistent state. Pre-freeze state is never migrated
// and an existing data directory is never reinterpreted as another profile.
func readControlPlaneProfile(dir string) (bool, error) {
	path := filepath.Join(dir, "topology.json")
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false, fmt.Errorf("invalid control-plane topology file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var profile struct {
		Version int   `json:"version"`
		HA      *bool `json:"ha"`
	}
	if err := json.Unmarshal(raw, &profile); err != nil {
		return false, fmt.Errorf("decode control-plane topology: %w", err)
	}
	if profile.Version != 1 || profile.HA == nil {
		return false, fmt.Errorf("unsupported control-plane topology; recreate the pre-freeze target")
	}
	return *profile.HA, nil
}

func ensureControlPlaneProfile(dir string, ha bool) error {
	existing, err := readControlPlaneProfile(dir)
	if err == nil {
		if existing != ha {
			return fmt.Errorf("control-plane topology conflict: existing ha=%t, requested ha=%t; use a separate target or explicitly destroy and recreate", existing, ha)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{composeName, envName} {
		if _, inspectErr := os.Lstat(filepath.Join(dir, name)); inspectErr == nil {
			return fmt.Errorf("control-plane topology missing in existing pre-freeze state; recreate the target")
		} else if !os.IsNotExist(inspectErr) {
			return inspectErr
		}
	}
	raw := fmt.Sprintf("{\"version\":1,\"ha\":%t}\n", ha)
	return os.WriteFile(filepath.Join(dir, "topology.json"), []byte(raw), 0600)
}

func (files Files) PostgresMembers() []string {
	if files.HA {
		return []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"}
	}
	return []string{"postgres-member-1"}
}

func (files Files) OpenBaoMembers() []string {
	if files.HA {
		return []string{"openbao-member-1", "openbao-member-2", "openbao-member-3"}
	}
	return []string{"openbao-member-1"}
}

func validateSingleControlPlane(rendered string) (string, error) {
	for _, forbidden := range []string{"spilo", "postgres-etcd", "member-2", "member-3", "/openbao/file", "/openbao/raft"} {
		if strings.Contains(rendered, forbidden) {
			return "", fmt.Errorf("single control-plane contains forbidden cluster/storage: %s", forbidden)
		}
	}
	for _, required := range []string{"docker.io/library/postgres:18-alpine", "docker.io/openbao/openbao:2.7.0", "ssl=on", "ssl_key_file=/tmp/server-key.pem", "hba_file=", "no-new-privileges:true", "postgres-data-1:/var/lib/postgresql", "./providers/openbao/runtime/openbao.hcl:/run/baseharbor/openbao/openbao.hcl:ro"} {
		if !strings.Contains(rendered, required) {
			return "", fmt.Errorf("single control-plane lacks security/storage projection: %s", required)
		}
	}
	return rendered, nil
}
