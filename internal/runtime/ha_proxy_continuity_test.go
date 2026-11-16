package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlPlaneHAProxiesRefreshMemberAddressesAndCloseDemotedSessions(t *testing.T) {
	for name, write := range map[string]func(string) error{
		"postgres": writeControlPlanePostgresHAProxyConfig,
		"openbao":  writeOpenBaoHAProxyConfig,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := write(dir); err != nil {
				t.Fatal(err)
			}
			configDir := dir
			if name == "openbao" {
				configDir = filepath.Join(dir, "providers", "openbao", "runtime")
			}
			data, err := os.ReadFile(filepath.Join(configDir, "haproxy.cfg"))
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"on-marked-down shutdown-sessions", "resolvers runtime-dns", "parse-resolv-conf"} {
				if !strings.Contains(string(data), required) {
					t.Fatalf("%s HA proxy does not preserve continuity: missing %s", name, required)
				}
			}
		})
	}
}
