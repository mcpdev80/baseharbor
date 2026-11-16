package identityprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestExistingKeycloakAccessNeverRewritesOrRepairsRuntimeProjection(t *testing.T) {
	for _, scenario := range []string{"existing", "missing", "redirected"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(root, "issuer"))
			if err != nil {
				t.Fatal(err)
			}
			manifest := application.New("read-proof", "prod", false, false, false)
			manifest.Services.Identity = true
			files, err := EnsureKeycloakFilesAt(context.Background(), manifest, issuer, root, "isolated")
			if err != nil {
				t.Fatal(err)
			}
			projection := filepath.Join(files.Dir, "native-tls", "runtime")
			originalTime := time.Unix(946684800, 0)
			for _, name := range []string{"ca.pem", "server.pem", "server-key.pem"} {
				path := filepath.Join(projection, name)
				if err := os.Chtimes(path, originalTime, originalTime); err != nil {
					t.Fatal(err)
				}
			}
			key := filepath.Join(projection, "server-key.pem")
			sentinel := filepath.Join(t.TempDir(), "foreign-material")
			if err := os.WriteFile(sentinel, []byte("retain foreign owner bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			if scenario != "existing" {
				if err := os.Remove(key); err != nil {
					t.Fatal(err)
				}
				if scenario == "redirected" {
					if err := os.Symlink(sentinel, key); err != nil {
						t.Fatal(err)
					}
				}
			}
			existing, err := ExistingKeycloakFilesAt(manifest, root, "isolated")
			if err != nil || existing.Dir != files.Dir || existing.Project != files.Project {
				t.Fatalf("existing authority unavailable: %v", err)
			}
			// The complete read-only access projection remains usable using existing
			// validated authority files, without regenerating any runtime material.
			client, err := serviceaccess.NewHTTPClient(existing.AdminAccess.Material, false)
			if err != nil || client == nil {
				t.Fatal("existing admin trust was not returned")
			}
			for _, name := range []string{"ca.pem", "server.pem", "server-key.pem"} {
				path := filepath.Join(projection, name)
				info, err := os.Lstat(path)
				if name == "server-key.pem" && scenario == "missing" {
					if !os.IsNotExist(err) {
						t.Fatal("read repaired missing runtime private-key projection")
					}
					continue
				}
				if name == "server-key.pem" && scenario == "redirected" {
					if err != nil || info.Mode()&os.ModeSymlink == 0 {
						t.Fatal("read replaced redirected projection")
					}
					continue
				}
				if err != nil || !info.ModTime().Equal(originalTime) || info.Mode().Perm() != 0644 {
					t.Fatal("read rewrote or changed projection metadata")
				}
			}
			after, err := os.ReadFile(sentinel)
			if err != nil || string(after) != "retain foreign owner bytes" {
				t.Fatal("read overwrote foreign bytes through runtime projection")
			}
		})
	}
}
