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
	assertValkeyReplicasUseNumericPrimary(t, compose)
	if got := strings.Count(compose, "printf 'protected-mode no\\n'"); got != 3 {
		t.Fatalf("Valkey HA compose has %d Sentinel protected-mode overrides, want 3:\n%s", got, compose)
	}
	if got := strings.Count(compose, "VALKEYCLI_AUTH=\"$$VALKEY_PASSWORD\" valkey-cli -h valkey -p 6379 --raw CLIENT INFO"); got != 5 {
		t.Fatalf("Valkey HA compose resolves the original primary IP through authenticated client metadata %d times, want 5:\n%s", got, compose)
	}
	if got := strings.Count(compose, "until [ -n \"$$primary_ip\" ]; do"); got != 5 {
		t.Fatalf("Valkey HA compose waits for primary readiness %d times, want 5:\n%s", got, compose)
	}
	if got := strings.Count(compose, "attempt=$$((attempt+1)); [ \"$$attempt\" -lt 60 ] || exit 1; sleep 1"); got != 5 {
		t.Fatalf("Valkey HA compose bounds member/Sentinel bootstrap retries %d times, want 5:\n%s", got, compose)
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
	if got := strings.Count(compose, "valkey-cli ping 2>&1 | grep -Eq '^PONG$|^NOAUTH '"); got != 3 {
		t.Fatalf("Valkey HA compose has %d credential-independent member liveness checks, want 3:\n%s", got, compose)
	}
	if strings.Contains(compose, "VALKEYCLI_AUTH=\"$${VALKEY_PASSWORD}\" valkey-cli ping") {
		t.Fatalf("Valkey HA member healthcheck must not depend on rotating credential projection:\n%s", compose)
	}
}

func TestSharedValkeyHAReplicasBootstrapWithNumericPrimary(t *testing.T) {
	var b strings.Builder
	writeSharedValkeyCompose(&b, sharedBackendAppState{
		Application: "demo", Environment: "dev",
		Cache: map[string]sharedValkeyResource{"default": {Instances: 3}},
	}, "default")
	assertValkeyReplicasUseNumericPrimary(t, b.String())
}

func assertValkeyReplicasUseNumericPrimary(t *testing.T, compose string) {
	t.Helper()
	if got := strings.Count(compose, `printf 'replicaof %s 6379\n' "$$primary_ip"`); got != 2 {
		t.Fatalf("Valkey HA requires two replicas with resolved primary IP, got %d:\n%s", got, compose)
	}
	if strings.Contains(compose, "printf 'replicaof valkey") {
		t.Fatalf("Valkey replicas must not resolve the stopped primary from their event loop:\n%s", compose)
	}
}
