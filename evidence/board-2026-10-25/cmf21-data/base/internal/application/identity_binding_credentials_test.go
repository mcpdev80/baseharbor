package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func identityCredentialProjectionFixture(t *testing.T) (Manifest, RuntimeFiles, IdentityDiscovery) {
	t.Helper()
	m := New("credential-projection", "dev", false, false, false)
	m.Services.Identity = true
	files := RuntimeFilesFor(Store{Root: t.TempDir()}, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	d := IdentityDiscovery{Issuer: "https://id.example", AuthorizationEndpoint: "https://id.example/auth", TokenEndpoint: "https://id.example/token", UserinfoEndpoint: "https://id.example/user", JWKSURI: "https://id.example/jwks"}
	if err := MaterializeIdentityBindingMaterial(m, files, "external-oidc", d, "client", "private-test-value", []byte("TEST-CA")); err != nil {
		t.Fatal(err)
	}
	return m, files, d
}

func TestIdentityBindingDetectsDivergentCredentialDelivery(t *testing.T) {
	for _, changed := range []string{"workload-secret", "runtime-secret", "application-secret-ref", "workload-trust", "runtime-trust", "client-id", "missing-secret"} {
		t.Run(changed, func(t *testing.T) {
			m, files, _ := identityCredentialProjectionFixture(t)
			if err := VerifyIdentityBinding(m, files); err != nil {
				t.Fatal(err)
			}
			host := filepath.Join(files.Bindings, "identity")
			workload := filepath.Join(workloadServiceBindingProjectionDir(files), "identity")
			switch changed {
			case "workload-secret":
				if err := writeReadOnlyProjectionFile(filepath.Join(workload, "client-secret"), []byte("divergent\n")); err != nil {
					t.Fatal(err)
				}
			case "runtime-secret", "runtime-trust", "client-id":
				values, err := readRuntimeEnv(files.Env)
				if err != nil {
					t.Fatal(err)
				}
				key := map[string]string{"runtime-secret": "IDENTITY_CLIENT_SECRET", "runtime-trust": "IDENTITY_CA_FILE", "client-id": "IDENTITY_CLIENT_ID"}[changed]
				values[key] = "divergent"
				if err := writeRuntimeEnv(files.Env, m, values); err != nil {
					t.Fatal(err)
				}
			case "application-secret-ref":
				values, err := loadApplicationEnvValues(files.ApplicationEnv)
				if err != nil {
					t.Fatal(err)
				}
				values["OIDC_CLIENT_SECRET_FILE"] = "/foreign/secret"
				if err := writeApplicationEnvValues(files.ApplicationEnv, values); err != nil {
					t.Fatal(err)
				}
			case "workload-trust":
				if err := writeReadOnlyProjectionFile(filepath.Join(workload, "ca.crt"), []byte("foreign\n")); err != nil {
					t.Fatal(err)
				}
			case "missing-secret":
				if err := os.Remove(filepath.Join(host, "client-secret")); err != nil {
					t.Fatal(err)
				}
			}
			err := VerifyIdentityBinding(m, files)
			if err == nil {
				t.Fatal("divergent/stale credential binding verified")
			}
			if strings.Contains(err.Error(), "private-test-value") {
				t.Fatal("credential leaked in verification error")
			}
		})
	}
}

func TestIdentityBindingCannotDropExistingCredentialOrTrustByOmission(t *testing.T) {
	m, files, discovery := identityCredentialProjectionFixture(t)
	before, _ := os.ReadFile(files.Env)
	for _, missing := range []string{"secret", "trust"} {
		secret, trust := "private-test-value", []byte("TEST-CA")
		if missing == "secret" {
			secret = ""
		} else {
			trust = nil
		}
		if err := MaterializeIdentityBindingMaterial(m, files, "external-oidc", discovery, "changed-client", secret, trust); err == nil {
			t.Fatal("implicit authentication/trust downgrade allowed")
		}
		after, _ := os.ReadFile(files.Env)
		if string(before) != string(after) {
			t.Fatal("rejected change partially rewrote credential authority")
		}
		if err := VerifyIdentityBinding(m, files); err != nil {
			t.Fatal(err)
		}
	}
}
