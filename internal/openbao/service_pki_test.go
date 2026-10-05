package openbao

import (
	"context"
	"encoding/json"
	"errors"
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
	case strings.Contains(joined, "sys/leader"):
		return "true\n", nil
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
	files := bhruntime.Files{HA: true, Env: filepath.Join(dir, "runtime.env"), Compose: filepath.Join(dir, "compose.yaml")}
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
		DNSNames:   []string{"prometheus-access", "localhost"},
		TTL:        24 * time.Hour,
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

type transientServiceCAFaker struct {
	servicePKIFake
	failures int
}

func (f *transientServiceCAFaker) ExecProjectInput(ctx context.Context, project, compose, env string, input []byte, service string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "baseharbor-pki/cert/ca") && f.failures > 0 {
		f.failures--
		return "", errors.New("transient EOF")
	}
	return f.servicePKIFake.ExecProjectInput(ctx, project, compose, env, input, service, args...)
}

func TestServiceCARetriesTransientReadFailure(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &transientServiceCAFaker{failures: 2}
	ca, err := ServiceCA(context.Background(), fake, files)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ca), "BEGIN CERTIFICATE") {
		t.Fatalf("unexpected CA material after retry: %s", ca)
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

type servicePKIRotationFake struct {
	activeCA       string
	defaultIssuer  string
	configPayloads []string
}

func (f *servicePKIRotationFake) ExecProject(_ context.Context, _, _, _, _ string, args ...string) (string, error) {
	return "", nil
}

func (f *servicePKIRotationFake) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, _ string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "auth/approle/login"):
		return `{"auth":{"client_token":"manager-token"}}`, nil
	case strings.Contains(joined, "sys/leader"):
		return "true\n", nil
	case strings.Contains(joined, "baseharbor-pki/root/rotate/internal"):
		return `{"data":{"issuer_id":"rotated-root"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/config/issuers"):
		parts := strings.SplitN(string(input), "\n", 2)
		if len(parts) != 2 {
			return "", nil
		}
		f.configPayloads = append(f.configPayloads, strings.TrimSpace(parts[1]))
		var payload map[string]string
		if err := json.Unmarshal([]byte(parts[1]), &payload); err == nil {
			f.defaultIssuer = payload["default"]
			if f.defaultIssuer == "rotated-root" {
				f.activeCA = "NEW-CA"
			}
		}
		return `{"data":{"default":"rotated-root"}}`, nil
	case strings.Contains(joined, "baseharbor-pki/cert/ca"):
		ca := f.activeCA
		if ca == "" {
			ca = "OLD-CA"
		}
		return `{"data":{"certificate":"-----BEGIN CERTIFICATE-----\n` + ca + `\n-----END CERTIFICATE-----"}}`, nil
	default:
		return "", nil
	}
}

func TestRotateServiceCAActivatesRotatedIssuerAsDefault(t *testing.T) {
	files := servicePKITestFiles(t)
	fake := &servicePKIRotationFake{activeCA: "OLD-CA"}
	if err := RotateServiceCA(context.Background(), fake, files); err != nil {
		t.Fatal(err)
	}
	if fake.defaultIssuer != "rotated-root" {
		t.Fatalf("default issuer = %q, want rotated-root", fake.defaultIssuer)
	}
	if fake.activeCA != "NEW-CA" {
		t.Fatalf("active CA = %q, want NEW-CA", fake.activeCA)
	}
	if len(fake.configPayloads) != 1 || !strings.Contains(fake.configPayloads[0], `"default":"rotated-root"`) {
		t.Fatalf("unexpected default-issuer payloads: %#v", fake.configPayloads)
	}
}
