package identityprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"go.yaml.in/yaml/v3"
)

func TestKeycloakComposeInheritsManagementHTTPS(t *testing.T) {
	app := application.WithHA(application.New("demo", "dev", false, false, false), true)
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
		"PGROOT: /home/postgres/pgroot",
		"PGDATA: /home/postgres/pgroot/data",
		"keycloak-db-data-1:/home/postgres/pgroot",
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

func TestKeycloakHAPostgresHAProxyUsesRuntimeDNS(t *testing.T) {
 dir:=t.TempDir()
 if err:=ensureKeycloakPostgresHA(dir);err!=nil{t.Fatal(err)}
 config,err:=os.ReadFile(filepath.Join(dir,"db-ha","haproxy.cfg"))
 if err!=nil{t.Fatal(err)}
 got:=string(config)
 for _,required:=range []string{"resolvers container_dns","parse-resolv-conf","resolvers container_dns resolve-prefer ipv4 init-addr last,none"} {
  if !strings.Contains(got,required) {t.Fatalf("HAProxy must refresh DNS after runtime recovery: missing %q",required)}
 }
}

func TestKeycloakSingleComposeHasNoHAResources(t *testing.T) {
 app := application.New("demo", "prod", false, false, false)
 files := KeycloakFiles{Project:"test",ConsumerNetwork:"test-identity",InternalNetwork:"test-identity-internal"}
 got := keycloakCompose(app,files)
 for _,want:=range []string{"  keycloak-1:","  keycloak-db:","  keycloak-db-init:","  keycloak-db-data:"} {
  if !strings.Contains(got,want) {t.Errorf("single mode missing %q",want)}
 }
 for _,unwanted:=range []string{"  keycloak-2:","  keycloak-3:","keycloak-db-member-1:","keycloak-db-etcd-1:","keycloak-db-tls:/run/baseharbor/db-tls:ro"} {
  if strings.Contains(got,unwanted) && unwanted != "keycloak-db-tls:/run/baseharbor/db-tls:ro" { t.Errorf("non-HA contains %q",unwanted) }
 }
 if strings.Count(got,"  keycloak-1:")!=1 {t.Error("single mode has duplicate member")}
}

func TestKeycloakTopologyComposeYAMLValid(t *testing.T) {
 for _,ha:=range []bool{false,true} {
  app:=application.WithHA(application.New("demo","prod",false,false,false),ha)
  files:=KeycloakFiles{Project:"owned",ConsumerNetwork:"owned-consumer",InternalNetwork:"owned-internal"}
  var document struct {
   Services map[string]yaml.Node `yaml:"services"`
   Volumes map[string]yaml.Node `yaml:"volumes"`
  }
  if err:=yaml.Unmarshal([]byte(keycloakCompose(app,files)),&document);err!=nil {t.Fatalf("HA=%v invalid compose: %v",ha,err)}
  count:=1
  if ha {count=3}
  for n:=1;n<=3;n++ {
   name:=fmt.Sprintf("keycloak-%d",n)
   _,ok:=document.Services[name]
   if ok!=(n<=count) {t.Errorf("HA=%v service %s present=%v",ha,name,ok)}
  }
  _,etcd:=document.Services["keycloak-db-etcd-1"]
  _,single:=document.Volumes["keycloak-db-data"]
  if etcd!=ha || single==ha {t.Errorf("HA=%v unexpected SQL realization etcd=%v singleVolume=%v",ha,etcd,single)}
 }
}
