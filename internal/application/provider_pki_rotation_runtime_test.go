package application

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func assertOldCARootRejected(t *testing.T, caPEM []byte, port string) {
	t.Helper()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("old CA test bundle is invalid")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(loopbackHost, port), &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: loopbackHost,
	})
	if err == nil {
		_ = conn.Close()
		t.Fatal("retired CA still validates the rotated TLS endpoint")
	}
}

type providerPKIRotationRuntime struct {
	bhruntime.RuntimeProvider
	service string
	args    []string
}

func (r *providerPKIRotationRuntime) ExecProject(_ context.Context, _, _, _, service string, args ...string) (string, error) {
	r.service = service
	r.args = append([]string(nil), args...)
	return "", nil
}

func TestReloadMongoDBCertificatesKeepsLeadingHyphenPasswordAsOptionValue(t *testing.T) {
	runtime := &providerPKIRotationRuntime{}
	manifest := Manifest{HA: true, Services: Services{DocumentDatabase: true}}
	files := RuntimeFiles{Project: "project", Compose: "compose", Env: "env"}
	if err := reloadMongoDBCertificates(context.Background(), runtime, manifest, files, "default"); err != nil {
		t.Fatal(err)
	}
	if runtime.service == "" || len(runtime.args) != 3 || runtime.args[0] != "sh" || runtime.args[1] != "-ec" {
		t.Fatalf("unexpected MongoDB certificate reload invocation: service=%q args=%q", runtime.service, runtime.args)
	}
	script := runtime.args[2]
	if !strings.Contains(script, `--username="$MONGO_INITDB_ROOT_USERNAME"`) || !strings.Contains(script, `--password="$MONGO_INITDB_ROOT_PASSWORD"`) {
		t.Fatalf("MongoDB auth options must use equals form so leading-hyphen credentials stay option values: %s", script)
	}
}
