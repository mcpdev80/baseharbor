package application

import (
	"github.com/mcpdev80/baseharbor/internal/capability"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedApplicationDataTopologyCannotBeImplicitlyReconciled(t *testing.T) {
	t.Setenv(ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
	t.Setenv(ProviderScopeEnv(capability.ProviderValkey), "application")
	for _, provider := range []string{"postgres", "valkey", "rabbitmq", "mongodb"} {
		t.Run(provider, func(t *testing.T) {
			m := New("demo", "dev", false, false, false)
			switch provider {
			case "postgres":
				m.Services.SQL = true
			case "valkey":
				m.Services.Cache = true
			case "rabbitmq":
				m.Services.MessagingQueue = true
			case "mongodb":
				m.Services.DocumentDatabase = true
			}
			files := RuntimeFiles{Compose: filepath.Join(t.TempDir(), "compose.yaml")}
			source := []byte("services:\n  " + provider + ": {}\n  " + provider + "-admin: {}\n  " + provider + "-access: {}\n")
			if err := os.WriteFile(files.Compose, source, 0600); err != nil {
				t.Fatal(err)
			}
			if err := CheckRuntimeTopology(files, m); err != nil {
				t.Fatal(err)
			}
			m.HA = true
			if err := CheckRuntimeTopology(files, m); err == nil || !strings.Contains(err.Error(), "topology change blocked") {
				t.Fatalf("silent migration permitted: %v", err)
			}
			after, _ := os.ReadFile(files.Compose)
			if string(after) != string(source) {
				t.Fatal("read-only guard changed retained data state")
			}
		})
	}
}
func TestSharedValkeyRejectsExistingMemberCountChanges(t *testing.T) {
	m := New("demo", "dev", false, true, false)
	state := sharedBackendState{Applications: map[string]sharedBackendAppState{sharedBackendApplicationKey(m): {Cache: map[string]sharedValkeyResource{defaultServiceInstance: {Instances: 3}}}}}
	if err := checkSharedValkeyTopology(state, m); err == nil {
		t.Fatal("retained Sentinel topology silently downgraded")
	}
	m.HA = true
	if err := checkSharedValkeyTopology(state, m); err != nil {
		t.Fatal(err)
	}
}
