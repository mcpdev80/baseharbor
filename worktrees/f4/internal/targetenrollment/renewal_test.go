package targetenrollment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type renewalMemoryStore struct {
	*memoryStore
	renewals int
}

func (s *renewalMemoryStore) CreateRenewal(ctx context.Context, grant Grant) error {
	s.renewals++
	return s.Create(ctx, grant)
}

func TestRenewalAuthorizationUsesSeparateStoreAndSharedOneUseCSRExchange(t *testing.T) {
	store := &renewalMemoryStore{memoryStore: newStore()}
	authority, err := New(store, serviceissuer.New(t))
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := authority.CreateRenewal(context.Background(), testScope(), time.Minute, time.Hour)
	if err != nil || store.renewals != 1 {
		t.Fatal("renewal bypassed explicit store authorization", err)
	}
	request := requestFor(t, testScope(), bootstrap)
	result, err := authority.Enroll(context.Background(), request)
	if err != nil || len(result.Certificate.PrivateKey) != 0 {
		t.Fatal("renewal changed canonical exchange or exposed node private material", err)
	}
	if _, err := authority.Enroll(context.Background(), request); err == nil {
		t.Fatal("renewal authorization was replayed")
	}
	initialOnly, _ := New(newStore(), serviceissuer.New(t))
	if _, err := initialOnly.CreateRenewal(context.Background(), testScope(), time.Minute, time.Hour); err == nil {
		t.Fatal("initial-only store silently performed renewal")
	}
}

func TestRenewalHTTPRequiresCoreUpdatePermissionAndScopeResolution(t *testing.T) {
	for _, role := range []string{"viewer", "unknown", "editor"} {
		store := &renewalMemoryStore{memoryStore: newStore()}
		authority, _ := New(store, serviceissuer.New(t))
		handler, _ := NewHTTP(authority, func(_ context.Context, target, node, environment string) (Scope, error) {
			if target != testScope().TargetID || node != testScope().NodeID || environment != "dev" {
				return Scope{}, ErrDenied
			}
			return testScope(), nil
		})
		request := grantHTTPRequest(role)
		request.URL.Path = RenewalAuthorizationPath
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if role != "editor" {
			if recorder.Code != http.StatusForbidden || store.renewals != 0 {
				t.Fatal("unprivileged caller created renewal authorization", role)
			}
			continue
		}
		if recorder.Code != http.StatusCreated || store.renewals != 1 || recorder.Header().Get("Cache-Control") != "no-store" ||
			contracts.ValidateTargetAccessRecord("bootstrap_authorization", recorder.Body.Bytes()) != nil {
			t.Fatal("authorized renewal changed the canonical protected grant", recorder.Code)
		}
		var bootstrap Bootstrap
		if err := json.Unmarshal(recorder.Body.Bytes(), &bootstrap); err != nil {
			t.Fatal(err)
		}
		exchange := httptest.NewRecorder()
		handler.ServeHTTP(exchange, enrollHTTPRequest(t, bootstrap))
		if exchange.Code != http.StatusOK {
			t.Fatal("renewal credential did not use the canonical one-use exchange", exchange.Code)
		}
	}
}

func TestInitialAndRenewalAuthorizationRejectAmbiguousInputBeforePersistence(t *testing.T) {
	valid := `{"target_id":"lab","node_id":"node-a","environment":"dev","lifetime_seconds":60,"certificate_ttl_seconds":3600}`
	for _, path := range []string{AuthorizationPath, RenewalAuthorizationPath} {
		for _, body := range []string{
			strings.Replace(valid, `"target_id":"lab"`, `"target_id":"foreign","target_id":"lab"`, 1),
			valid + `{}`, strings.Replace(valid, `"environment":"dev"`, "\"environment\":\"dev\xff\"", 1),
			`null`, strings.Replace(valid, `"lifetime_seconds":60`, `"lifetime_seconds":[]`, 1),
		} {
			handler, store := enrollmentHTTPFixture(t)
			request := grantHTTPRequest("editor")
			request.URL.Path = path
			request.Body = io.NopCloser(strings.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || len(store.grants) != 0 {
				t.Fatal("ambiguous authorization reached persistence", path, response.Code)
			}
		}
	}
}
