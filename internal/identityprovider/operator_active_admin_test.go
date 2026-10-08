package identityprovider

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestOperatorBootstrapUsesRotatedAdmin(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/master/.well-known/openid-configuration" {
			w.Write([]byte(`{}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("username") != "rotated-admin" || r.Form.Get("password") != "rotated-password" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Write([]byte(`{"access_token":"test-token"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	files := KeycloakFiles{Dir: dir, Env: filepath.Join(dir, "runtime.env"), AdminURL: server.URL}
	files.AdminAccess.Material = serviceaccess.TLSMaterial{CA: ca}
	if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_ADMIN_USER": "retired-admin", "BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD": "retired-password"}); err != nil {
		t.Fatal(err)
	}
	if err := replaceKeycloakAdminState(dir, "rotated-admin", "rotated-password"); err != nil {
		t.Fatal(err)
	}
	admin, err := operatorKeycloakAdmin(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if admin.user != "rotated-admin" || admin.token != "test-token" {
		t.Fatal("bootstrap did not use active admin")
	}
}
