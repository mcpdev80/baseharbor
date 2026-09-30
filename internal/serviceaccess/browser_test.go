package serviceaccess

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVerifyBrowserSurfaceFollowsSameAuthorityRedirects(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, server.URL+"/login", http.StatusFound)
		case "/login":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := VerifyBrowserSurface(context.Background(), server.Client(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyBrowserSurfaceRejectsDroppedFallbackPort(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, err := url.Parse(r.URL.String())
		if err != nil {
			t.Fatal(err)
		}
		target.Scheme = "https"
		target.Host = strings.Split(r.Host, ":")[0]
		target.Path = "/admin/"
		w.Header().Set("Location", target.String())
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	err := VerifyBrowserSurface(context.Background(), server.Client(), server.URL+"/")
	if err == nil || !strings.Contains(err.Error(), "changed canonical authority") {
		t.Fatalf("expected dropped-port authority error, got %v", err)
	}
}

func TestVerifyBrowserSurfaceRejectsExternalRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/login", http.StatusFound)
	}))
	defer server.Close()

	err := VerifyBrowserSurface(context.Background(), server.Client(), server.URL+"/")
	if err == nil || !strings.Contains(err.Error(), "changed canonical authority") {
		t.Fatalf("expected external redirect rejection, got %v", err)
	}
}

func TestVerifyBrowserSurfaceRejectsRedirectLoop(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("https://%s%s", r.Host, r.URL.Path), http.StatusFound)
	}))
	defer server.Close()

	err := VerifyBrowserSurface(context.Background(), server.Client(), server.URL+"/")
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected bounded redirect failure, got %v", err)
	}
}

func TestVerifyBrowserSurfaceRejectsNonSuccessfulFinalResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	err := VerifyBrowserSurface(context.Background(), server.Client(), server.URL+"/")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected semantic final-response failure, got %v", err)
	}
}
