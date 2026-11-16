package objectstorage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIAMClientCreateAccessKeyUsesSigV4AndDecodesSecret(t *testing.T) {
	var gotAction, gotAuth, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		gotAction = values.Get("Action")
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<CreateAccessKeyResponse><CreateAccessKeyResult><AccessKey><AccessKeyId>AKIATEST</AccessKeyId><SecretAccessKey>secret-value</SecretAccessKey></AccessKey></CreateAccessKeyResult></CreateAccessKeyResponse>`)
	}))
	defer server.Close()

	client, err := NewIAMClient(server.URL, server.Client(), AdminCredentials{AccessKeyID: "ADMINKEY", SecretAccessKey: "ADMINSECRET"})
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.CreateAccessKey(context.Background(), "runtime-user")
	if err != nil {
		t.Fatal(err)
	}
	if key.AccessKeyID != "AKIATEST" || key.SecretAccessKey != "secret-value" {
		t.Fatalf("unexpected key: %#v", key)
	}
	if gotAction != "CreateAccessKey" {
		t.Fatalf("Action = %q", gotAction)
	}
	if !strings.Contains(gotAuth, "Credential=ADMINKEY/") || !strings.Contains(gotAuth, "/iam/aws4_request") {
		t.Fatalf("IAM SigV4 authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
}

func TestIAMClientCreateUserTreatsEntityAlreadyExistsAsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `<ErrorResponse><Error><Code>EntityAlreadyExists</Code></Error></ErrorResponse>`)
	}))
	defer server.Close()

	client, err := NewIAMClient(server.URL, server.Client(), AdminCredentials{AccessKeyID: "ADMINKEY", SecretAccessKey: "ADMINSECRET"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CreateUser(context.Background(), "runtime-user"); err != nil {
		t.Fatalf("idempotent CreateUser: %v", err)
	}
}
