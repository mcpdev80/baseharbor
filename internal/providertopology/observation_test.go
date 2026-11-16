package providertopology

import (
	"github.com/mcpdev80/baseharbor/internal/availability"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservationSeparatesDataHelpersAndIndependentTSDBs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	data := `services:
  postgres-member-1: {image: docker.io/library/postgres:18}
  postgres: {image: docker.io/library/haproxy:3}
  postgres-admin: {image: docker.io/library/postgres:18}
  postgres-init: {image: docker.io/library/postgres:18}
  prometheus-1: {image: prom/prometheus:v3}
  prometheus-2: {image: prom/prometheus:v3}
  prometheus-access: {image: docker.io/library/haproxy:3}
  otel-collector-1: {image: otel/opentelemetry-collector-contrib:latest}
  otel-collector-2: {image: otel/opentelemetry-collector-contrib:latest}
  valkey: {image: docker.io/valkey/valkey:9}
  valkey-sentinel: {image: docker.io/valkey/valkey:9}
  valkey-access: {image: docker.io/library/haproxy:3}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	obs, err := Observe(path, []string{"postgres-member-1", "postgres", "postgres-admin", "prometheus-1", "prometheus-2", "prometheus-access", "otel-collector-1", "otel-collector-2", "valkey", "valkey-sentinel", "valkey-access"}, availability.Intent{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Observation{}
	for _, o := range obs {
		byName[o.Provider] = o
	}
	pg := byName["postgresql"]
	if pg.DataMembers != 1 || pg.Helpers != 2 || pg.DeclaredHelpers != 3 || strings.Contains(pg.Detail(), "ha-active=true") {
		t.Fatalf("helpers counted as database replicas: %#v", pg)
	}
	prom := byName["prometheus"]
	if prom.DataMembers != 2 || prom.Helpers != 1 || prom.Replication != "0-independent-tsdb" {
		t.Fatalf("independent TSDBs misclassified: %#v", prom)
	}
	otel := byName["otel-collector"]
	if otel.Members != 2 || otel.DataMembers != 0 || otel.Replication != "0-stateless" {
		t.Fatalf("receivers counted as data replicas: %#v", otel)
	}
	valkey := byName["valkey"]
	if valkey.DataMembers != 1 || valkey.Helpers != 2 {
		t.Fatalf("Sentinel/access counted as data: %#v", valkey)
	}
}

func TestIndependentLogicalInstancesAreNotReportedAsHA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	source := `services:
  valkey-one: {image: docker.io/valkey/valkey:9}
  valkey-one-2: {image: docker.io/valkey/valkey:9}
  valkey-one-access: {image: docker.io/library/haproxy:3}
  valkey-one-2-access: {image: docker.io/library/haproxy:3}
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	groups := map[string]string{"valkey-one": "cache/one", "valkey-one-2": "key_value/one-2"}
	result, err := Observe(path, []string{"valkey-one", "valkey-one-2", "valkey-one-access", "valkey-one-2-access"}, availability.Intent{}, groups)
	if err != nil || len(result) != 2 {
		t.Fatalf("independent instance inventory: %#v %v", result, err)
	}
	for _, o := range result {
		if o.DataMembers != 1 || o.Helpers != 1 || strings.Contains(o.Detail(), "ha-active=true") {
			t.Fatalf("independent instance counted as replica: %#v", o)
		}
	}
}

func TestRunningMembersRequireSemanticProofBeforeClaimingHA(t *testing.T) {
	o := Observation{Provider: "valkey", Members: 3, DataMembers: 3, DeclaredMembers: 3, RequestedHA: true, Replication: "configured-not-proven"}
	if detail := o.Detail(); !strings.Contains(detail, "ha-active=unknown") || strings.Contains(detail, "replicas=2") {
		t.Fatalf("container counts claimed replicated HA: %s", detail)
	}
	o.SemanticProof = "valkey-primary-replicas-sentinel"
	if detail := o.Detail(); !strings.Contains(detail, "ha-active=true") || !strings.Contains(detail, "replicas=2") || !strings.Contains(detail, "failover-proof=quorum-ready") {
		t.Fatalf("native primary/replica/Sentinel proof not projected: %s", detail)
	}
	o.Members, o.DataMembers = 2, 2
	if strings.Contains(o.Detail(), "ha-active=true") {
		t.Fatal("partial inventory reused complete cluster proof")
	}
	o.Provider, o.Members, o.DataMembers, o.SemanticProof = "rabbitmq", 3, 3, "rabbitmq-cluster-membership"
	if detail := o.Detail(); !strings.Contains(detail, "replicas=queue-policy-dependent") || strings.Contains(detail, "replicas=2") {
		t.Fatalf("cluster membership claimed replication for every queue: %s", detail)
	}
}
