package identityprovider

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type existingKeycloakReadOnlyRuntime struct{ mutations int }

func (r *existingKeycloakReadOnlyRuntime) ConfigProject(context.Context, string, string, string) error {
	r.mutations++
	return nil
}
func (r *existingKeycloakReadOnlyRuntime) UpProject(context.Context, string, string, string) error {
	r.mutations++
	return nil
}
func (r *existingKeycloakReadOnlyRuntime) DestroyProject(context.Context, string, string, string) error {
	r.mutations++
	return nil
}

func TestExistingIdentityNeverMaterializesMissingDevelopmentGateway(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(root, "authority"))
	if err != nil {
		t.Fatal(err)
	}
	app := application.New("read-only", "dev", false, false, false)
	app.Services.Identity = true
	if _, err := EnsureKeycloakFilesAt(context.Background(), app, issuer, root, "isolated"); err != nil {
		t.Fatal(err)
	}
	before := existingIdentityFileSnapshot(t, root)
	runtime := &existingKeycloakReadOnlyRuntime{}
	realization := newLocalKeycloakRealization(runtime, app, issuer, root, "isolated")
	if _, err := realization.Existing(context.Background()); err == nil {
		t.Fatal("missing gateway trust was reported ready")
	}
	after := existingIdentityFileSnapshot(t, root)
	if runtime.mutations != 0 || !reflect.DeepEqual(before, after) {
		for path, digest := range after {
			if previous, exists := before[path]; !exists || previous != digest {
				t.Logf("readiness modified existing owner path: %s", path)
			}
		}
		t.Fatal("existing/readiness path materialized or rewrote runtime/trust state")
	}
}

func TestExistingProductionIdentityReturnsCertificateBytesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(root, "authority"))
	if err != nil {
		t.Fatal(err)
	}
	app := application.New("read-only", "prod", false, false, false)
	app.Services.Identity = true
	if _, err := EnsureKeycloakFilesAt(context.Background(), app, issuer, root, "isolated"); err != nil {
		t.Fatal(err)
	}
	before := existingIdentityFileSnapshot(t, root)
	runtime := &existingKeycloakReadOnlyRuntime{}
	realization := newLocalKeycloakRealization(runtime, app, issuer, root, "isolated")
	instance, err := realization.Existing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(instance.TrustBundle)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("Identity trust contains a filename instead of a certificate")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		t.Fatal("Identity trust certificate is invalid")
	}
	if runtime.mutations != 0 || !reflect.DeepEqual(before, existingIdentityFileSnapshot(t, root)) {
		t.Fatal("existing production Identity inspection changed runtime/trust state")
	}
}

func existingIdentityFileSnapshot(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = sha256.Sum256(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}
