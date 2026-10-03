package runtime

import (
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
	if err := writeOpenBaoRuntimeConfig(dir, user, secret); err != nil {
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


func TestWriteOpenBaoHAProxyConfigRoutesOnlyToActiveLeader(t *testing.T) {
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
		"/v1/sys/health?standbyok=false&perfstandbyok=false",
		"http-check expect status 200,501,503",
		"default-server check check-ssl verify none",
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("OpenBao HAProxy config missing leader-aware health check %q:\n%s", required, config)
		}
	}
	if strings.Contains(config, "balance roundrobin") || strings.Contains(config, "option tcp-check") {
		t.Fatalf("OpenBao HAProxy must not route writes to healthy standbys:\n%s", config)
	}
}
