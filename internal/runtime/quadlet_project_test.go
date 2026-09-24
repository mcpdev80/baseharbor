package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderComposeProjectQuadletsMapsManagedRuntimeSemantics(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	env := filepath.Join(root, "runtime.env")
	if err := os.WriteFile(env, []byte("DB_PORT=15432\nDB_PASSWORD=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := `services:
  db:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    user: "postgres"
    read_only: true
    cap_drop: ["ALL"]
    cap_add: ["NET_BIND_SERVICE"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    ports:
      - "127.0.0.1:${DB_PORT}:5432"
    volumes:
      - db-data:/var/lib/postgresql
      - ./config:/etc/example:ro
    networks:
      - internal
    healthcheck:
      test: ["CMD-SHELL", "echo ok"]
      interval: 2s
      timeout: 3s
      retries: 4
      start_period: 1s
  worker:
    image: docker.io/library/alpine:3.22
    command: ["sh", "-c", "sleep 60"]
    depends_on:
      - db
    networks:
      - internal
volumes:
  db-data:
networks:
  internal:
    internal: true
`
	if err := os.Mkdir(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compose, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectQuadlets(compose, env, "baseharbor-demo")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{
		"baseharbor-demo-db.container",
		"baseharbor-demo-worker.container",
		"baseharbor-demo-db-data.volume",
		"baseharbor-demo-internal.network",
		"baseharbor-demo-db.env",
	} {
		if _, ok := got.Files[file]; !ok {
			t.Fatalf("missing generated file %s", file)
		}
	}

	db := got.Files["baseharbor-demo-db.container"]
	for _, want := range []string{
		"Image=docker.io/library/postgres:18-alpine",
		"User=postgres",
		"ReadOnly=true",
		"DropCapability=all",
		"AddCapability=NET_BIND_SERVICE",
		"NoNewPrivileges=true",
		"Tmpfs=/tmp:rw,noexec,nosuid,nodev",
		"PublishPort=127.0.0.1:15432:5432",
		"Volume=baseharbor-demo-db-data.volume:/var/lib/postgresql",
		"Network=baseharbor-demo-internal.network",
		"NetworkAlias=db",
		"HealthInterval=2s",
		"HealthTimeout=3s",
		"HealthRetries=4",
		"HealthStartPeriod=1s",
	} {
		if !strings.Contains(db, want) {
			t.Fatalf("db Quadlet missing %q:\n%s", want, db)
		}
	}
	if strings.Contains(db, "Notify=healthy") {
		t.Fatalf("Quadlet service start must not duplicate BaseHarbor readiness waits:\n%s", db)
	}
	if !strings.Contains(got.Files["baseharbor-demo-internal.network"], "Internal=true") {
		t.Fatalf("internal network lost semantics:\n%s", got.Files["baseharbor-demo-internal.network"])
	}
	for name, content := range map[string]string{
		"network": got.Files["baseharbor-demo-internal.network"],
		"volume":  got.Files["baseharbor-demo-db-data.volume"],
	} {
		for _, want := range []string{
			"Label=com.docker.compose.project=baseharbor-demo",
			"Label=io.podman.compose.project=baseharbor-demo",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s Quadlet missing project ownership label %q:\n%s", name, want, content)
			}
		}
	}
	worker := got.Files["baseharbor-demo-worker.container"]
	for _, want := range []string{
		"Requires=baseharbor-demo-db.service",
		"After=baseharbor-demo-db.service",
		"Exec=sh -c \"sleep 60\"",
	} {
		if !strings.Contains(worker, want) {
			t.Fatalf("worker Quadlet missing %q:\n%s", want, worker)
		}
	}
}

func TestRenderComposeProjectQuadletsSkipsInactiveProfilesUnlessSelected(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(compose, []byte(`services:
  app:
    image: docker.io/library/alpine:3.22
  standalone:
    image: docker.io/library/alpine:3.22
    profiles: ["standalone"]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectQuadlets(compose, "", "profile-default")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Files["profile-default-app.container"]; !ok {
		t.Fatal("default service was not rendered")
	}
	if _, ok := got.Files["profile-default-standalone.container"]; ok {
		t.Fatal("inactive profile service must not be rendered by default")
	}

	selected, err := RenderComposeProjectQuadlets(compose, "", "profile-selected", "standalone")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := selected.Files["profile-selected-standalone.container"]; !ok {
		t.Fatal("explicitly selected profile service was not rendered")
	}
	if _, ok := selected.Files["profile-selected-app.container"]; ok {
		t.Fatal("unselected default service must not be rendered for explicit selection")
	}
}

func TestExpandQuadletComposeStringSupportsComposeDefaultsAndDollarEscape(t *testing.T) {
	env := map[string]string{"SET": "value"}
	got := expandQuadletComposeString("a=${SET} b=${MISSING:-fallback} c=$$TOKEN", env)
	if got != "a=value b=fallback c=$TOKEN" {
		t.Fatalf("expanded = %q", got)
	}
}

func TestRenderComposeProjectFilesQuadletsMergesBaseHarborOverride(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "compose.yaml")
	override := filepath.Join(root, "override.yaml")
	if err := os.WriteFile(base, []byte(`services:
  api:
    image: docker.io/library/alpine:3.22
    ports:
      - "8080:8080"
    environment:
      ORIGINAL: base
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte(`services:
  api:
    environment:
      MANAGED: yes
    networks:
      baseharbor-backend:
        aliases:
          - api-metrics
networks:
  baseharbor-backend:
    external: true
    name: baseharbor-demo-backend
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectFilesQuadlets([]string{base, override}, "", "baseharbor-workload-demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	api := got.Files["baseharbor-workload-demo-api.container"]
	for _, want := range []string{
		"PublishPort=8080:8080",
		"Network=baseharbor-demo-backend",
		"NetworkAlias=api-metrics",
		"EnvironmentFile=./baseharbor-workload-demo-api.env",
	} {
		if !strings.Contains(api, want) {
			t.Fatalf("merged api Quadlet missing %q:\n%s", want, api)
		}
	}
	env := got.Files["baseharbor-workload-demo-api.env"]
	if !strings.Contains(env, "ORIGINAL=base") || !strings.Contains(env, "MANAGED=yes") {
		t.Fatalf("merged environment incomplete:\n%s", env)
	}
}

func TestRenderComposeProjectQuadletsMapsFileSecrets(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "broker.key")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(root, "compose.yaml")
	data := `services:
  broker:
    image: ghcr.io/example/runtime:test
    secrets:
      - broker-key
secrets:
  broker-key:
    file: ./broker.key
`
	if err := os.WriteFile(compose, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectQuadlets(compose, "", "baseharbor-broker")
	if err != nil {
		t.Fatal(err)
	}
	unit := got.Files["baseharbor-broker-broker.container"]
	want := "Volume=" + secret + ":/run/secrets/broker-key:ro"
	if !strings.Contains(unit, want) {
		t.Fatalf("Quadlet secret mapping missing %q:\n%s", want, unit)
	}
}

func TestRenderComposeProjectQuadletsCreatesImplicitDefaultNetwork(t *testing.T) {
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(compose, []byte(`services:
  api:
    image: docker.io/library/alpine:3.22
`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectQuadlets(compose, "", "implicit-network")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Files["implicit-network-default.network"]; !ok {
		t.Fatalf("implicit default network was not rendered")
	}
	unit := got.Files["implicit-network-api.container"]
	if !strings.Contains(unit, "Network=implicit-network-default.network") {
		t.Fatalf("service was not attached to implicit default network:\n%s", unit)
	}
}

func TestQuadletSystemdJoinPreservesContainerDollarExpansion(t *testing.T) {
	got := quadletSystemdJoin([]string{"sh", "-ec", "echo $VALKEY_PASSWORD && echo ${OTHER}"})
	for _, want := range []string{"$$VALKEY_PASSWORD", "${OTHER}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("systemd command lost escaped dollar %q: %s", want, got)
		}
	}
}

func TestRenderQuadletHealthCommandUsesNativeShellExpression(t *testing.T) {
	got, err := renderQuadletHealthCommand([]string{"CMD-SHELL", "pg_isready -U baseharbor -d app"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "pg_isready -U baseharbor -d app" {
		t.Fatalf("CMD-SHELL health command = %q", got)
	}
}

func TestRenderQuadletHealthCommandEscapesSystemdDollarExpansion(t *testing.T) {
	got, err := renderQuadletHealthCommand([]string{"CMD-SHELL", `VALKEYCLI_AUTH="$VALKEY_PASSWORD" valkey-cli ping | grep -q '^PONG$'`})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"$$VALKEY_PASSWORD", "^PONG$$"} {
		if !strings.Contains(got, want) {
			t.Fatalf("health command missing %q: %s", want, got)
		}
	}
}

func TestRenderComposeProjectQuadletsMapsLongFormSecretTarget(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "token")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(root, "compose.yaml")
	data := `services:
  api:
    image: docker.io/library/alpine:3.22
    secrets:
      - source: service-token
        target: baseharbor-runtime-token
secrets:
  service-token:
    file: ./token
`
	if err := os.WriteFile(compose, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := RenderComposeProjectQuadlets(compose, "", "baseharbor-demo")
	if err != nil {
		t.Fatal(err)
	}
	unit := got.Files["baseharbor-demo-api.container"]
	want := "Volume=" + secret + ":/run/secrets/baseharbor-runtime-token:ro"
	if !strings.Contains(unit, want) {
		t.Fatalf("long-form secret target missing %q:\n%s", want, unit)
	}
}
