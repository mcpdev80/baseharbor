package identityprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRealmReconcileReadsBackFailedCreationBeforeUpdating(t *testing.T) {
	desired := keycloakRealm{Realm: "core", Enabled: true, Attributes: map[string]string{"owner": "installation"}}
	created, reads, posts, puts := false, 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			reads++
			if !created {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(desired)
		case http.MethodPost:
			posts++
			created = true
			w.WriteHeader(http.StatusInternalServerError)
		case http.MethodPut:
			puts++
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.reconcileRealm(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || posts != 1 || puts != 1 {
		t.Fatalf("ambiguous creation was replayed: reads=%d posts=%d puts=%d", reads, posts, puts)
	}
}

func TestRealmReconcileRejectsForeignOwnershipAfterServerFailure(t *testing.T) {
	desired := keycloakRealm{Realm: "core", Attributes: map[string]string{"owner": "installation"}}
	reads, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads++
			current := desired
			if reads > 1 {
				current.Attributes = map[string]string{"owner": "foreign"}
			}
			json.NewEncoder(w).Encode(current)
			return
		}
		writes++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
	err := admin.reconcileRealm(context.Background(), desired)
	if err == nil || !strings.Contains(err.Error(), "not owned") || reads != 2 || writes != 1 {
		t.Fatalf("foreign realm modified: reads=%d writes=%d err=%v", reads, writes, err)
	}
}

func TestRealmReconcileDoesNotRetryWriteAccessDenial(t *testing.T) {
	desired := keycloakRealm{Realm: "core", Attributes: map[string]string{"owner": "installation"}}
	reads, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads++
			json.NewEncoder(w).Encode(desired)
			return
		}
		writes++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	admin := keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.reconcileRealm(context.Background(), desired); err == nil || reads != 1 || writes != 1 {
		t.Fatalf("access denial replayed: reads=%d writes=%d err=%v", reads, writes, err)
	}
}
