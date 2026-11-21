package coreupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	"github.com/mcpdev80/baseharbor/internal/dcspki"
)

// TestDCSBootProbeInheritsParentEnv proves that the etcd boot probe's child
// process (etcdctl) inherits the parent environment. The stub writes the env it
// receives to a file; the test sets a card-scoped sentinel before the call and
// asserts the child side sees it. This is criterion 3 of card 0428-222.
func TestDCSBootProbeInheritsParentEnv(t *testing.T) {
	const sentinelKey = "BASEHARBOR_0428_222_SENTINEL"
	const sentinelVal = "1"
	root := t.TempDir()
	out := filepath.Join(root, "child_env.txt")

	pki, err := dcspki.Ensure(filepath.Join(root, "pki"), []string{"localhost"})
	if err != nil {
		t.Fatal(err)
	}

	prev, had := os.LookupEnv(sentinelKey)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(sentinelKey, prev)
		} else {
			_ = os.Unsetenv(sentinelKey)
		}
	})
	if err := os.Setenv(sentinelKey, sentinelVal); err != nil {
		t.Fatal(err)
	}

	endpoints := []string{"https://one:2379", "https://two:2379", "https://three:2379"}
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	// Capture the inherited env once, for whichever endpoint we are called with.
	script.WriteString("env | grep -v '^PATH=' > " + quoteShell(out) + "\n")
	script.WriteString("case \"$1\" in\n")
	for i, endpoint := range endpoints {
		status := map[string]any{"header": map[string]any{"cluster_id": 456, "member_id": i + 1, "revision": 100}, "leader": 1, "version": "3.7.2"}
		data, err := json.Marshal([]map[string]any{{"Endpoint": endpoint, "Status": status}})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&script, "--endpoints=%s) printf '%%s\\n' '%s' ;;\n", endpoint, data)
	}
	script.WriteString("*) exit 1 ;;\nesac\n")

	binary := filepath.Join(root, "etcdctl")
	if err := os.WriteFile(binary, []byte(script.String()), 0700); err != nil {
		t.Fatal(err)
	}

	proof := 0
	probe := EtcdBootProbe{Binary: binary, Endpoints: endpoints, TLS: etcdbackup.TLSFiles{CA: pki.CA, Cert: pki.ClientCert, Key: pki.ClientKey}, VerifyPatroniDCS: func(context.Context) error { proof++; return nil }}
	if err := probe.Verify(context.Background(), etcdbackup.Identity{Core: "core", Target: "target", Cluster: "123"}, etcdbackup.SnapshotInfo{ClusterID: "123", Revision: 100, Version: "3.7.2"}); err != nil {
		t.Fatalf("boot probe failed: %v", err)
	}
	assertSentinelInCoreupdateEnvFile(t, out, sentinelKey, sentinelVal)
}

func assertSentinelInCoreupdateEnvFile(t *testing.T, path, key, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("did not capture child env: %v", err)
	}
	for _, line := range splitLines(string(data)) {
		if line == key+"="+want {
			return
		}
	}
	t.Fatalf("sentinel %s=%s not inherited by child; captured env:\n%s", key, want, string(data))
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func quoteShell(s string) string {
	return "'" + replaceSingleQuote(s) + "'"
}

func replaceSingleQuote(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b = append(b, '\'', '\\', '\'')
		} else {
			b = append(b, s[i])
		}
	}
	return string(b)
}
