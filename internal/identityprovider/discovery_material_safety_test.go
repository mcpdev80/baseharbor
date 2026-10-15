package identityprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoveryRejectsCredentialURLsWithoutDisclosure(t *testing.T) {
	for _, field := range []string{"issuer", "authorization_endpoint", "token_endpoint", "userinfo_endpoint", "jwks_uri", "end_session_endpoint"} {
		t.Run(field, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				doc := map[string]string{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "userinfo_endpoint": server.URL + "/userinfo", "jwks_uri": server.URL + "/jwks"}
				doc[field] = "https://user:private-url-sentinel@issuer.example.test/endpoint"
				_ = json.NewEncoder(w).Encode(doc)
			}))
			defer server.Close()
			_, err := FetchDiscovery(t.Context(), server.Client(), server.URL)
			if err == nil || strings.Contains(err.Error(), "private-url-sentinel") {
				t.Fatal("credential-bearing discovery URL was admitted or disclosed")
			}
		})
	}
}

func TestDiscoverySeparatesPublicQueryFromCredentialQuery(t *testing.T) {
	for _, query := range []string{"tenant=public", "access_token=private-url-sentinel", "client_secret=private-url-sentinel"} {
		t.Run(query, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token?" + query, "userinfo_endpoint": server.URL + "/userinfo", "jwks_uri": server.URL + "/jwks"})
			}))
			defer server.Close()
			_, err := FetchDiscovery(t.Context(), server.Client(), server.URL)
			if query == "tenant=public" {
				if err != nil {
					t.Fatal("ordinary public endpoint query was permanently forbidden")
				}
				return
			}
			if err == nil || strings.Contains(err.Error(), "private-url-sentinel") {
				t.Fatal("credential query was admitted or disclosed")
			}
		})
	}
}
