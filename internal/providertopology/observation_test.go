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
