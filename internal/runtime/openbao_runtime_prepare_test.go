package runtime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWriteOpenBaoRuntimeConfigEncodesPostgresCredentialsAsURLUserinfo(t *testing.T) {
	dir := t.TempDir()
	const (
		user   = "openbao+runtime@example"
		secret = "p@ss:word/+with?reserved#chars"
	)
	if err := writeOpenBaoRuntimeConfig(dir, user, secret, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "providers", "openbao", "runtime", "openbao.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`connection_url\s*=\s*"([^"]+)"`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatalf("OpenBao PostgreSQL connection URL missing from config: %s", data)
	}
	parsed, err := url.Parse(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.User.Username(); got != user {
		t.Fatalf("decoded PostgreSQL username = %q, want %q", got, user)
	}
	gotSecret, ok := parsed.User.Password()
	if !ok || gotSecret != secret {
		t.Fatalf("decoded PostgreSQL password = %q ok=%v, want exact original", gotSecret, ok)
	}
	if parsed.Host != "postgres:5432" || parsed.Path != "/openbao" {
		t.Fatalf("unexpected PostgreSQL storage URL target: %s", parsed.String())
	}
}

func TestWriteOpenBaoHAProxyConfigRoutesToActiveMember(t *testing.T) {
	dir := t.TempDir()
	if err := writeOpenBaoHAProxyConfig(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "providers", "openbao", "runtime", "haproxy.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, required := range []string{
		"option httpchk",
		"/v1/sys/health",
		"http-check expect status 200",
		"default-server check check-ssl verify none",
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("OpenBao HAProxy config missing active-member health check %q:\n%s", required, config)
		}
	}
	for _, forbidden := range []string{"standbyok=true", "perfstandbyok=true", "option tcp-check"} {
		if strings.Contains(config, forbidden) {
			t.Fatalf("OpenBao HAProxy must route control-plane writes only to the active member; found %q:\n%s", forbidden, config)
		}
	}
}

// TestOpenBaoPostgresInitGrantsStorageRoleConnect verifies the generated
// openbao-init.sh grants the non-superuser OpenBao backend role CONNECT on its
// storage database as well as the postgres/template1 databases it may probe on
// startup, without elevating it to Superuser. This is the regression guard for
// the "permission denied for database" OpenBao backend FATAL.
func TestOpenBaoPostgresInitGrantsStorageRoleConnect(t *testing.T) {
	dir := t.TempDir()
	if err := writeOpenBaoPostgresInit(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "providers", "postgresql", "runtime", "openbao-init.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, required := range []string{
		// Owner + explicit grant on the storage database so the role can connect
		// even after a prior partial run left the database owned by another role.
		"ALTER DATABASE openbao OWNER TO",
		"GRANT CONNECT ON DATABASE openbao TO",
		// The backend may probe the default/database-listed databases on startup.
		"GRANT CONNECT, TEMPORARY ON DATABASE postgres TO",
		"GRANT CONNECT, TEMPORARY ON DATABASE template1 TO",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("openbao-init.sh missing required privilege grant %q:\n%s", required, script)
		}
	}
	// The OpenBao backend role must remain a restricted login, never a Superuser.
	for _, forbidden := range []string{
		"CREATE ROLE %I LOGIN SUPERUSER",
		"ALTER ROLE %I WITH LOGIN SUPERUSER",
	} {
		if strings.Contains(script, fmt.Sprintf(forbidden, "openbao_user")) {
			t.Fatalf("openbao-init.sh must not make the backend role a Superuser:\n%s", script)
		}
	}
}
