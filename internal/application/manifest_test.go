package application

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	want := WithRequiredSecrets(New("mailflow", "prod", true, true, true), "SMTP_PASSWORD", "OPENAI_API_KEY")
	got, err := ParseYAML(want.YAML())
	if err != nil {
		t.Fatal(err)
	}
	expected := WithRequiredSecrets(New("mailflow", "prod", true, true, true), "OPENAI_API_KEY", "SMTP_PASSWORD")
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, expected)
	}
	if !strings.Contains(want.YAML(), "    - name: OPENAI_API_KEY\n") {
		t.Fatalf("canonical YAML does not use explicit required secret objects:\n%s", want.YAML())
	}
}

func TestManifestNamedServiceInstancesRoundTrip(t *testing.T) {
	want := New("mailflow", "prod", false, false, false)
	want.Services.Postgres = false
	want = WithPostgresInstances(want, "primary", "analytics")
	want = WithRedisInstances(want, "cache", "sessions")
	got, err := ParseYAML(want.YAML())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(PostgresInstanceNames(got), []string{"analytics", "primary"}) {
		t.Fatalf("unexpected PostgreSQL instances: %#v", PostgresInstanceNames(got))
	}
	if !reflect.DeepEqual(RedisInstanceNames(got), []string{"cache", "sessions"}) {
		t.Fatalf("unexpected Redis instances: %#v", RedisInstanceNames(got))
	}
	for _, expected := range []string{"    instances:\n", "      primary: {}\n", "      analytics: {}\n", "      cache: {}\n", "      sessions: {}\n"} {
		if !strings.Contains(got.YAML(), expected) {
			t.Fatalf("named service YAML missing %q:\n%s", expected, got.YAML())
		}
	}
}

func TestManifestSingleServiceKeepsCompactCompatibility(t *testing.T) {
	m := New("demo", "dev", true, true, false)
	if got := PostgresInstanceNames(m); !reflect.DeepEqual(got, []string{"default"}) {
		t.Fatalf("unexpected default PostgreSQL instances: %#v", got)
	}
	if got := RedisInstanceNames(m); !reflect.DeepEqual(got, []string{"default"}) {
		t.Fatalf("unexpected default Redis instances: %#v", got)
	}
	if strings.Contains(m.YAML(), "instances:") {
		t.Fatalf("simple manifest should stay compact:\n%s", m.YAML())
	}
}

func TestManifestParsesLegacyScalarRequiredSecrets(t *testing.T) {
	input := `version: 1
app:
  name: demo
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: true
secrets:
  required:
    - API_TOKEN
`
	m, err := ParseYAML(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := RequiredSecretNames(m); !reflect.DeepEqual(got, []string{"API_TOKEN"}) {
		t.Fatalf("unexpected legacy required secrets %#v", got)
	}
}

func TestManifestDefaultsToPostgres(t *testing.T) {
	m := New("demo", "", false, false, false)
	if m.Environment != "dev" || !m.Services.Postgres {
		t.Fatalf("unexpected defaults: %#v", m)
	}
}

func TestRequiredSecretsEnableManagedSecrets(t *testing.T) {
	m := WithRequiredSecrets(New("demo", "dev", true, false, false), "API_TOKEN")
	if !m.Services.Secrets {
		t.Fatal("required secret declaration must enable managed secrets")
	}
}

func TestManifestValidationFailsClosed(t *testing.T) {
	cases := []Manifest{
		New("UPPER", "dev", true, false, false),
		{Version: 99, Name: "demo", Environment: "dev", Services: Services{Postgres: true}},
		{Version: 1, Name: "demo", Environment: "dev"},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Postgres: true}, Secrets: SecretRequirements{Required: []SecretRequirement{{Name: "API_TOKEN"}}}},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Secrets: true}, Secrets: SecretRequirements{Required: []SecretRequirement{{Name: "bad/key"}}}},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Secrets: true}, Secrets: SecretRequirements{Required: []SecretRequirement{{Name: "API_TOKEN"}, {Name: "API_TOKEN"}}}},
		{Version: 1, Name: "demo", Environment: "dev", Services: Services{Postgres: true, PostgresInstances: map[string]ServiceInstance{"Bad_Name": {}}}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validation failure for %#v", tc)
		}
	}
}

func TestStoreCreateLoadList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "apps")
	store := Store{Root: root}
	for _, name := range []string{"zeta", "alpha"} {
		path, err := store.Create(New(name, "dev", true, false, false))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("manifest permissions = %o", info.Mode().Perm())
		}
	}
	m, _, err := store.Load("alpha")
	if err != nil || m.Name != "alpha" {
		t.Fatalf("load failed: %#v %v", m, err)
	}
	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("unexpected list: %#v", items)
	}
	_, err = store.Create(New("alpha", "dev", true, false, false))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("expected ErrExists, got %v", err)
	}
}

func TestParserRejectsUnknownFields(t *testing.T) {
	_, err := ParseYAML(strings.Replace(New("demo", "dev", true, false, false).YAML(), "app:\n", "app:\n  surprise: yes\n", 1))
	if err == nil {
		t.Fatal("expected unknown field to fail")
	}
}

func TestManifestHTTPExposureRoundTrip(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m = WithWorkload(m, "compose.yaml", "web")
	m = WithHTTPExposure(m, "public", "web", 8080, "http")

	got, err := ParseYAML(m.YAML())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Exposures, []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http", Visibility: "public"}}) {
		t.Fatalf("unexpected exposures: %#v", got.Exposures)
	}
	for _, want := range []string{"exposure:\n", "  http:\n", "    - name: public\n", "      service: web\n", "      port: 8080\n", "      protocol: http\n", "      visibility: public\n"} {
		if !strings.Contains(got.YAML(), want) {
			t.Fatalf("exposure YAML missing %q:\n%s", want, got.YAML())
		}
	}
}

func TestManifestHTTPExposureRequiresExplicitSelectedService(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m.Workload = WorkloadConfig{Compose: "compose.yaml"}
	m.Exposures = []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http"}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "explicit workload.services") {
		t.Fatalf("expected deterministic workload service validation, got %v", err)
	}

	m.Workload.Services = []string{"api"}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "not selected") {
		t.Fatalf("expected target selection validation, got %v", err)
	}
}

func TestManifestHTTPExposureVisibilityFailsClosed(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m.Workload = WorkloadConfig{Compose: "compose.yaml", Services: []string{"web"}}
	m.Exposures = []HTTPExposureRequirement{{Name: "public", Service: "web", Port: 8080, Protocol: "http", Visibility: "private-ish"}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "visibility must be public or internal") {
		t.Fatalf("expected invalid visibility rejection, got %v", err)
	}
}

func TestManifestYAMLOmitsDisabledServices(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	yaml := m.YAML()
	if !strings.Contains(yaml, "  postgres:\n    enabled: true\n") {
		t.Fatalf("enabled PostgreSQL missing from sparse YAML:\n%s", yaml)
	}
	for _, unwanted := range []string{"redis:", "secrets:", "object_storage:", "enabled: false"} {
		if strings.Contains(yaml, unwanted) {
			t.Fatalf("sparse YAML contains disabled capability %q:\n%s", unwanted, yaml)
		}
	}

	legacy := `version: 1
app:
  name: demo
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: false
`
	parsed, err := ParseYAML(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Services.Postgres || parsed.Services.Redis || parsed.Services.Secrets {
		t.Fatalf("legacy explicit false flags changed semantics: %#v", parsed.Services)
	}
}


func TestManifestRuntimePermissionsRoundTrip(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.Postgres = false
	m = WithWorkload(m, "compose.yaml", "api")
	m = WithRuntimePermission(m, "object-storage.s3/v1", "runtime.create", "runtime.get", "runtime.delete")

	got, err := ParseYAML(m.YAML())
	if err != nil {
		t.Fatal(err)
	}
	if !RuntimePermissionFor(got, "object-storage.s3/v1", "runtime.create") ||
		!RuntimePermissionFor(got, "object-storage.s3/v1", "runtime.get") ||
		!RuntimePermissionFor(got, "object-storage.s3/v1", "runtime.delete") {
		t.Fatalf("runtime permissions lost in round trip: %#v", got.Runtime.Permissions)
	}
	if RuntimePermissionFor(got, "object-storage.s3/v1", "runtime.rotate") {
		t.Fatal("undeclared runtime.rotate was granted")
	}
	for _, want := range []string{
		"runtime:\n",
		"  permissions:\n",
		"    - capability: object-storage.s3/v1\n",
		"      operations:\n",
		"        - runtime.create\n",
	} {
		if !strings.Contains(got.YAML(), want) {
			t.Fatalf("runtime YAML missing %q:\n%s", want, got.YAML())
		}
	}
}

func TestManifestRuntimePermissionsFailClosed(t *testing.T) {
	base := New("demo", "dev", true, false, false)
	cases := []RuntimePermission{
		{Capability: "object-storage.s3/v9", Operations: []string{"runtime.create"}},
		{Capability: "object-storage.s3/v1", Operations: nil},
		{Capability: "object-storage.s3/v1", Operations: []string{"create"}},
		{Capability: "object-storage.s3/v1", Operations: []string{"runtime.create", "runtime.create"}},
	}
	for _, permission := range cases {
		m := base
		m.Runtime.Permissions = []RuntimePermission{permission}
		if err := m.Validate(); err == nil {
			t.Fatalf("expected invalid runtime permission to fail: %#v", permission)
		}
	}
}
