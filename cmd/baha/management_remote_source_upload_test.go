package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/sourcetransfer"
)

// This is the HTTPS client/bytes/receipt round trip. Authorization is tested
// independently in machinehttp using its real OIDC policy boundary.
func TestRemoteSourceUploadDeltaRoundTripOverTLS(t *testing.T) {
	store, err := sourcetransfer.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-session" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/machine/source-transfers":
			if r.URL.Query().Get("target") != "local" {
				http.Error(w, "workstation alias leaked to Core", 400)
				return
			}
			m, _, err := store.Current(r.URL.Query().Get("target"), r.URL.Query().Get("application"), r.URL.Query().Get("environment"))
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(struct {
				Digest string `json:"digest"`
			}{m.Digest})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/machine/source-transfers":
			var m sourcetransfer.Manifest
			if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
				http.Error(w, "bad manifest", 400)
				return
			}
			id, missing, err := store.Begin(m)
			if err != nil {
				http.Error(w, "begin failure", 409)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(struct {
				ID      string   `json:"id"`
				Missing []string `json:"missing"`
			}{id, missing})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v1/machine/source-transfers/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/machine/source-transfers/")
			var body struct {
				Path   string `json:"path"`
				Offset int64  `json:"offset"`
				Data   []byte `json:"data"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || store.Put(id, body.Path, body.Offset, body.Data) != nil {
				http.Error(w, "bad chunk", 409)
				return
			}
			io.WriteString(w, "{}")
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/commit"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/machine/source-transfers/"), "/commit")
			m, _, err := store.Commit(id)
			if err != nil {
				http.Error(w, "commit failure", 409)
				return
			}
			json.NewEncoder(w).Encode(struct {
				Digest string `json:"digest"`
			}{m.Digest})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := &verifiedRemoteCore{client: server.Client(), origin: server.URL, token: "test-session", target: "dev-docker-1", coreTarget: "local", tenant: "tenant-a"}
	root := t.TempDir()
	for p, data := range map[string]string{"baseharbor.yaml": "name: demo\n", "src/main.go": "package main\n", "new.txt": "untracked"} {
		file := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	first, err := c.uploadLocalSource(context.Background(), "dev-docker-1", "demo", "dev", root)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("empty remote source digest")
	}
	again, err := c.uploadLocalSource(context.Background(), "dev-docker-1", "demo", "dev", root)
	if err != nil || again != first {
		t.Fatalf("unchanged must preserve snapshot identity: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/main.go"), []byte("package main // dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := c.uploadLocalSource(context.Background(), "dev-docker-1", "demo", "dev", root)
	if err != nil || second == first {
		t.Fatalf("delta must advance digest: %v", err)
	}
	_, dir, err := store.Current("local", "demo", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted file survived")
	}
	source, err := os.ReadFile(filepath.Join(dir, "src/main.go"))
	if err != nil || !bytes.Contains(source, []byte("dirty")) {
		t.Fatal("dirty source did not arrive")
	}
}
