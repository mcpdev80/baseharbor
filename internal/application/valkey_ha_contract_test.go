package application

import (
	"fmt"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestValkeyHAComposeEnablesSentinelQuorumOnRuntimeNetwork(t *testing.T) {
	useApplicationScopedDataProviders(t)
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
	assertValkeySentinelInternalNetwork(t, compose, "valkey", "default")
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
	if got := strings.Count(compose, "sentinel monitor baseharbor valkey 6379 2"); got != 3 {
		t.Fatalf("Valkey HA compose has %d stable member Sentinel quorum monitors, want 3:\n%s", got, compose)
	}
	assertValkeySentinelRetainsMemberNames(t, compose)
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
	app := sharedBackendAppState{
		Application: "demo", Environment: "dev",
		Cache: map[string]sharedValkeyResource{"default": {Instances: 3}},
	}
	b.WriteString("services:\n")
	writeSharedValkeyCompose(&b, app, "default")
	b.WriteString("networks:\n  shared-backend: {}\n")
	writeSharedValkeyHANetworks(&b, sharedBackendState{Applications: map[string]sharedBackendAppState{"demo/dev": app}}, []string{"demo/dev"})
	assertValkeyReplicasUseNumericPrimary(t, b.String())
	assertValkeySentinelRetainsMemberNames(t, b.String())
	assertValkeySentinelInternalNetwork(t, b.String(), sharedValkeyMemberServiceName(app, "default", 0), "shared-backend")
}

func assertValkeySentinelInternalNetwork(t *testing.T, compose, primary, applicationNetwork string) {
	t.Helper()
	var model struct {
		Services map[string]struct {
			Networks map[string]any `yaml:"networks"`
		} `yaml:"services"`
		Networks map[string]struct {
			Internal bool `yaml:"internal"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal([]byte(compose), &model); err != nil {
		t.Fatal(err)
	}
	network := primary + "-ha"
	if !model.Networks[network].Internal {
		t.Fatal("Sentinel's member discovery network must answer missing internal names without external DNS forwarding")
	}
	for ordinal := 0; ordinal < 3; ordinal++ {
		suffix := ""
		if ordinal > 0 {
			suffix = fmt.Sprintf("-%d", ordinal+1)
		}
		sentinel := model.Services[primary+"-sentinel"+suffix].Networks
		if _, ok := sentinel[network]; !ok || len(sentinel) != 1 {
			t.Fatalf("Sentinel must use only its internal HA network, got %v", sentinel)
		}
		member := model.Services[primary+suffix].Networks
		if _, ok := member[network]; !ok {
			t.Fatalf("member is unreachable from Sentinel's internal network: %v", member)
		}
		if _, ok := member[applicationNetwork]; !ok {
			t.Fatalf("member lost its application gateway network: %v", member)
		}
	}
}

func assertValkeySentinelRetainsMemberNames(t *testing.T, compose string) {
	t.Helper()
	for _, directive := range []string{"sentinel resolve-hostnames yes", "sentinel announce-hostnames no", "replica-announce-ip "} {
		if got := strings.Count(compose, directive); got != 3 {
			t.Fatalf("Valkey HA must retain all stable member identities while Sentinel sends numeric replication addresses: %q count=%d, want 3", directive, got)
		}
	}
	if strings.Contains(compose, "sentinel announce-hostnames yes") {
		t.Fatal("Sentinel must not configure replicas to resolve primary hostnames from their event loops")
	}
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
