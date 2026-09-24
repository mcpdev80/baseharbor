package openbao

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type servicePKIFake struct {
	inputs []string
	args   []string
}

func (f *servicePKIFake) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	f.args = append(f.args, strings.Join(args, " "))
	return "", nil
}

func (f *servicePKIFake) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	f.args = append(f.args, joined)
	f.inputs = append(f.inputs, string(input))
	switch {
	case strings.Contains(joined, "auth/approle/login"):
		return `{"auth":{"client_token":"manager-token"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/cert/ca"):
		return `{"data":{"certificate":"-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/issue/baseharbor-services"):
		return `{"data":{"certificate":"-----BEGIN CERTIFICATE-----\nLEAF\n-----END CERTIFICATE-----","private_key":"-----BEGIN PRIVATE KEY-----\nKEY\n-----END PRIVATE KEY-----","issuing_ca":"-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----","ca_chain":["-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----"],"serial_number":"01:02:03","expiration":1893456000}}`, nil
	case strings.Contains(joined, "baseharbor-pki/revoke"):
		return `{"data":{}}`, nil
	default:
		return "", nil
	}
}

func servicePKITestFiles(t *testing.T) bhruntime.Files {
	t.Helper()
	dir := t.TempDir()
	files := bhruntime.Files{Env: filepath.Join(dir, "runtime.env"), Compose: filepath.Join(dir, "compose.yaml")}
	if err := os.WriteFile(AdminCredentialsPath(files), []byte("OPENBAO_ROLE_ID=role\nOPENBAO_SECRET_ID=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestIssueServiceCertificateUsesOpenBaoPKIWithoutSecretArguments(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &servicePKIFake{}
	cert, err := IssueServiceCertificate(context.Background(), fake, files, ServiceCertificateRequest{
		CommonName: "prometheus.baseharbor",
		DNSNames: []string{"prometheus-access", "localhost"},
		TTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cert.Serial != "01:02:03" || len(cert.Certificate) == 0 || len(cert.PrivateKey) == 0 || len(cert.IssuingCA) == 0 {
		t.Fatalf("unexpected certificate material: %#v", cert)
	}
	for _, args := range fake.args {
		if strings.Contains(args, "manager-token") || strings.Contains(args, "PRIVATE KEY") {
			t.Fatalf("secret material leaked into command arguments: %q", args)
		}
	}
	foundIssue := false
	for i, args := range fake.args {
		if strings.Contains(args, "baseharbor-pki/issue/baseharbor-services") {
			foundIssue = true
			parts := strings.SplitN(fake.inputs[i], "\n", 2)
			if len(parts) != 2 {
				t.Fatalf("expected token plus JSON payload, got %q", fake.inputs[i])
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(parts[1]), &payload); err != nil {
				t.Fatalf("invalid issue payload: %v", err)
			}
			if payload["common_name"] != "prometheus.baseharbor" {
				t.Fatalf("unexpected common_name: %#v", payload)
			}
		}
	}
	if !foundIssue {
		t.Fatal("OpenBao PKI issue endpoint was not called")
	}
}

func TestServiceCAReadsPublicCAOnly(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &servicePKIFake{}
	ca, err := ServiceCA(context.Background(), fake, files)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ca), "BEGIN CERTIFICATE") || strings.Contains(string(ca), "PRIVATE KEY") {
		t.Fatalf("unexpected CA material: %s", ca)
	}
}
