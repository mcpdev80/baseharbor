package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

func TestRegisteredOIDCVerificationRejectsDifferentDeliveredAuthenticationRoutes(t *testing.T) {
	for _, field := range []string{"oidc.token-endpoint", "oidc.jwks-uri", "oidc.authorization-endpoint", "oidc.userinfo-endpoint", "oidc.issuer"} {
		for _, both := range []bool{false, true} {
			name := field + "/workload"
			if both {
				name = field + "/host-and-workload"
			}
			t.Run(name, func(t *testing.T) {
				r, _, _ := restoreRegisteredIdentityFixture(t, true)
				files := application.RuntimeFilesFor(r.Store, r.Manifest)
				driver, err := identityprovider.NewRegisteredExternalDriver(r.Manifest, files, r.TargetStateRoot)
				if err != nil {
					t.Fatal(err)
				}
				if err := driver.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
					t.Fatal(err)
				}
				if err := driver.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
					t.Fatal("unchanged delivered authentication failed verification", err)
				}
				fresh, err := identityprovider.NewRegisteredExternalDriver(r.Manifest, files, r.TargetStateRoot)
				if err != nil || fresh.Verify(t.Context(), capability.Resource{}, capability.Binding{}) != nil {
					t.Fatal("fresh read-only driver could not verify valid existing delivery", err)
				}
				roots := []string{application.WorkloadServiceBindingProjectionDir(files)}
				if both {
					roots = append(roots, files.Bindings)
				}
				for _, root := range roots {
					path := filepath.Join(root, application.IdentityBindingName, field)
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("https://different-receiver.example.test/authentication\n"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, info.Mode().Perm()); err != nil {
						t.Fatal(err)
					}
				}

				if err := driver.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
					t.Fatal("different delivered authentication route falsely verified against original discovery")
				}
				if err := fresh.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
					t.Fatal("fresh verification accepted delivery that differs from current discovery")
				}
			})
		}
	}
}

func TestRegisteredOIDCVerificationRejectsDifferentDeliveredIssuerEnvironment(t *testing.T) {
	for _, workload := range []bool{false, true} {
		name := "runtime"
		if workload {
			name = "application"
		}
		t.Run(name, func(t *testing.T) {
			r, _, _ := restoreRegisteredIdentityFixture(t, true)
			files := application.RuntimeFilesFor(r.Store, r.Manifest)
			driver, err := identityprovider.NewRegisteredExternalDriver(r.Manifest, files, r.TargetStateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err := driver.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			if err := driver.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			path, key := files.Env, "IDENTITY_CONTAINER_ISSUER="
			if workload {
				path, key = files.ApplicationEnv, "OIDC_ISSUER="
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines, changed := strings.Split(string(data), "\n"), false
			for i, line := range lines {
				if strings.HasPrefix(line, key) {
					lines[i], changed = key+"https://different-receiver.example.test/issuer", true
				}
			}
			if !changed {
				t.Fatal("fixture did not deliver the workload issuer")
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
				t.Fatal(err)
			}
			if err := driver.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
				t.Fatal("different issuer environment falsely verified against original discovery")
			}
		})
	}
}
