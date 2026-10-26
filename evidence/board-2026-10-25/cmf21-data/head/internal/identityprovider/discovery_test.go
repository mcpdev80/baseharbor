package identityprovider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchDiscoveryRetriesTransientServerFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		issuer := "https://identity.example/realms/demo"
		_, _ = io.WriteString(w, `{"issuer":"`+issuer+`","authorization_endpoint":"`+issuer+`/auth","token_endpoint":"`+issuer+`/token","userinfo_endpoint":"`+issuer+`/userinfo","jwks_uri":"`+issuer+`/certs"}`)
	}))
	defer server.Close()

	issuer := "https://identity.example/realms/demo"
	got, err := FetchDiscoveryAt(context.Background(), server.Client(), server.URL, issuer)
	if err != nil || got.Issuer != issuer || attempts != 2 {
		t.Fatalf("transient discovery = %#v, attempts %d, error %v", got, attempts, err)
	}
}

func TestFetchDiscoveryPersistentServerFailureHonorsDeadline(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := FetchDiscoveryAt(ctx, server.Client(), server.URL, "https://identity.example/realms/demo")
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 {
		t.Fatalf("persistent discovery ignored deadline: attempts %d, error %v", attempts, err)
	}
}

func TestFetchDiscoveryDoesNotRetryRejectedAccess(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := FetchDiscoveryAt(context.Background(), server.Client(), server.URL, "https://identity.example/realms/demo")
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || attempts != 1 {
		t.Fatalf("rejected discovery retried: attempts %d, error %v", attempts, err)
	}
}
