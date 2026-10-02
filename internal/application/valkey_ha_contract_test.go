package application

import (
	"strings"
	"testing"
)

func TestValkeyHAComposeEnablesSentinelQuorumOnRuntimeNetwork(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "valkey-ha-contract",
		Environment:   "dev",
		HA:            true,
		Services: Services{
			KeyValue: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	compose, err := RuntimeComposeYAMLForProject(m, "bh-valkey-ha-contract")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(compose, "printf 'protected-mode no\\n'"); got != 3 {
		t.Fatalf("Valkey HA compose has %d Sentinel protected-mode overrides, want 3:\n%s", got, compose)
	}
	if got := strings.Count(compose, "sentinel monitor baseharbor valkey 6379 2"); got != 3 {
		t.Fatalf("Valkey HA compose has %d Sentinel quorum monitors, want 3:\n%s", got, compose)
	}
	if got := strings.Count(compose, "sentinel auth-pass baseharbor"); got != 3 {
		t.Fatalf("Valkey HA compose has %d Sentinel auth-pass entries, want 3:\n%s", got, compose)
	}
}
