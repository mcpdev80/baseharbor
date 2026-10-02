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
	if got := strings.Count(compose, "primary_ip=\"$(getent hosts valkey | awk 'NR==1 { print $1 }')\""); got != 3 {
		t.Fatalf("Valkey HA compose resolves the original primary IP %d times, want 3:\n%s", got, compose)
	}
	if got := strings.Count(compose, "sentinel monitor baseharbor %s 6379 2"); got != 3 {
		t.Fatalf("Valkey HA compose has %d numeric Sentinel quorum monitors, want 3:\n%s", got, compose)
	}
	if strings.Contains(compose, "sentinel resolve-hostnames yes") || strings.Contains(compose, "replica-announce-ip valkey-") {
		t.Fatalf("Valkey HA compose must not depend on stopped-container DNS for failover:\n%s", compose)
	}
	if got := strings.Count(compose, "sentinel auth-pass baseharbor"); got != 3 {
		t.Fatalf("Valkey HA compose has %d Sentinel auth-pass entries, want 3:\n%s", got, compose)
	}
}
