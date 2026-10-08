package identityprovider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyCoreUserInfoRejectsMissingAndForeignToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/master/protocol/openid-connect/userinfo" || r.Header.Get("Authorization") != "Bearer private-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"sub":"subject-123","preferred_username":"managed-admin"}`)
	}))
	defer server.Close()
	if err := verifyCoreUserInfo(context.Background(), server.Client(), server.URL, "private-token", "managed-admin"); err != nil {
		t.Fatal(err)
	}
	if err := verifyCoreUserInfo(context.Background(), server.Client(), server.URL, "wrong", "managed-admin"); err == nil {
		t.Fatal("unauthorized token accepted")
	}
	if err := verifyCoreUserInfo(context.Background(), server.Client(), server.URL, "private-token", "foreign-admin"); err == nil {
		t.Fatal("foreign user accepted")
	}
	if err := verifyCoreUserInfo(context.Background(), server.Client(), server.URL, "", "managed-admin"); err == nil {
		t.Fatal("missing token accepted")
	}
}
