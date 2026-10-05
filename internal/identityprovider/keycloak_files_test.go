package identityprovider

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestKeycloakComposeInheritsManagementHTTPS(t *testing.T) {
	app := application.New("demo", "dev", false, false, false)
	files := KeycloakFiles{
		Project:         "baseharbor-demo",
		ConsumerNetwork: "baseharbor-demo-identity",
		InternalNetwork: "baseharbor-demo-identity-internal",
	}

	got := keycloakCompose(app, files)

	if strings.Contains(got, "KC_HTTP_MANAGEMENT_SCHEME: https") {
		t.Fatalf("Keycloak compose contains invalid management scheme override:\n%s", got)
	}
	if !strings.Contains(got, "KC_HOSTNAME: ${BASEHARBOR_KEYCLOAK_CANONICAL_URL}") {
		t.Fatalf("Keycloak compose lost canonical hostname configuration:\n%s", got)
	}
	if strings.Contains(got, "KC_HOSTNAME_ADMIN:") {
		t.Fatalf("Keycloak compose must not force a separate admin hostname; the admin surface may redirect to the canonical public identity authority:\n%s", got)
	}
	if !strings.Contains(got, "--https-port=8443") {
		t.Fatalf("Keycloak compose missing native HTTPS port:\n%s", got)
	}
	if !strings.Contains(got, "keycloak-db-init:") || !strings.Contains(got, "condition: service_completed_successfully") {
		t.Fatalf("Keycloak compose must wait for the verified HA database bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "keycloak-db-member-1") || !strings.Contains(got, "keycloak-db-member-2") || !strings.Contains(got, "keycloak-db-member-3") {
		t.Fatalf("Keycloak compose must include the three Patroni database members:\n%s", got)
	}
	if !strings.Contains(got, "until psql -h keycloak-db -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null;") {
		t.Fatalf("Keycloak compose missing authenticated PostgreSQL TLS readiness probe:\n%s", got)
	}
	if !strings.Contains(got, "/dev/tcp/127.0.0.1/8443") {
		t.Fatalf("Keycloak compose missing native HTTPS listener health probe:\n%s", got)
	}
	for _, want := range []string{
		"keycloak-db-tls-init:",
		"keycloak-db-tls:/run/baseharbor/db-tls:ro",
		"uid=$$(id -u postgres); gid=$$(id -g postgres)",
		"chown \"$$uid:$$gid\"",
		"chmod 0600 /target/server-key.pem",
		"-v app_user=\"$$BASEHARBOR_KEYCLOAK_DB_USER\"",
		"attempts=$$((attempts+1))",
		"if [ \"$$attempts\" -ge 90 ]",
		"exists=$$(printf",
		"if [ \"$$exists\" != \"1\" ]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Keycloak HA compose missing protected TLS/bootstrap expression %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"-v app_user=\"$BASEHARBOR_KEYCLOAK_DB_USER\"",
		"attempts=$((attempts+1))",
		"if [ \"$attempts\" -ge 90 ]",
		"exists=$(psql",
		"if [ \"$exists\" != \"1\" ]",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Keycloak HA compose contains unescaped shell interpolation %q:\n%s", forbidden, got)
		}
	}
}
