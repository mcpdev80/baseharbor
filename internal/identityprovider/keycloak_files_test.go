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
	if !strings.Contains(got, "pg_isready") {
		t.Fatalf("Keycloak compose missing PostgreSQL readiness probe:\n%s", got)
	}
	if !strings.Contains(got, "/dev/tcp/127.0.0.1/8443") {
		t.Fatalf("Keycloak compose missing native HTTPS listener health probe:\n%s", got)
	}
}
