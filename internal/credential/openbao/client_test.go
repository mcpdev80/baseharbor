package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/credential"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestConfigRequiresHTTPS(t *testing.T) {
	_, err := New(Config{Address: "http://bao.example", Token: "token", Mount: "secret"})
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("expected ErrInvalidAddress, got %v", err)
	}
}

func TestResolveReadsKVv2SecretWithoutLeakingToken(t *testing.T) {
	cfg := Config{Address: "https://bao.example", Token: "super-secret-token", Namespace: "team-a", Mount: "secret"}
	client, err := NewWithHTTPClient(cfg, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://bao.example/v1/secret/data/apps/mailflow" {
			t.Fatalf("unexpected URL: %s", req.URL.String())
		}
		if req.Header.Get("X-Vault-Token") != cfg.Token {
			t.Fatal("missing token header")
		}
		if req.Header.Get("X-Vault-Namespace") != cfg.Namespace {
			t.Fatal("missing namespace header")
		}
		body := `{"data":{"data":{"username":"mailflow","password":"top-secret","port":5432}}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := client.Resolve(context.Background(), "apps/mailflow", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved.Payload["username"]) != "mailflow" || string(resolved.Payload["password"]) != "top-secret" {
		t.Fatalf("unexpected payload: %#v", resolved.Payload)
	}
	if string(resolved.Payload["port"]) != "5432" {
		t.Fatalf("expected JSON value for non-string field, got %q", resolved.Payload["port"])
	}

	raw, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "top-secret") || strings.Contains(string(raw), "mailflow") {
		t.Fatalf("secret leaked through JSON: %s", raw)
	}
}

func TestResolveMapsSafeErrors(t *testing.T) {
	for status, expected := range map[int]error{
		http.StatusForbidden:  ErrAccessDenied,
		http.StatusNotFound:   ErrNotFound,
		http.StatusBadGateway: ErrRequestFailed,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client, err := NewWithHTTPClient(Config{Address: "https://bao.example", Token: "secret-token", Mount: "secret"}, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"errors":["backend detail"]}`)), Header: make(http.Header)}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Resolve(context.Background(), "apps/test", "")
			if !errors.Is(err, expected) {
				t.Fatalf("expected %v, got %v", expected, err)
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "backend detail") {
				t.Fatalf("sensitive backend data leaked in error: %v", err)
			}
		})
	}
}

func TestResolveRejectsEmptyReference(t *testing.T) {
	client, err := NewWithHTTPClient(Config{Address: "https://bao.example", Token: "token", Mount: "secret"}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), "", "tenant-a")
	if !errors.Is(err, credential.ErrEmptyRef) {
		t.Fatalf("expected ErrEmptyRef, got %v", err)
	}
}
