package endpoint

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestHTTPPortScheme(t *testing.T) {
	for _, tt := range []struct {
		target, published int
		scheme            string
		ok                bool
	}{
		{443, 18443, "https", true},
		{8080, 18080, "http", true},
		{5432, 15432, "", false},
	} {
		got, ok := HTTPPortScheme(tt.target, tt.published)
		if got != tt.scheme || ok != tt.ok {
			t.Fatalf("HTTPPortScheme(%d,%d)=%q,%v want %q,%v", tt.target, tt.published, got, ok, tt.scheme, tt.ok)
		}
	}
}

func TestProbeHTTPAcceptsRedirectAndRejects5xx(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusPermanentRedirect)
	}))
	defer redirect.Close()
	host, port := hostPort(t, redirect.URL)
	got := ProbeHTTP(context.Background(), Endpoint{Service: "web", Scheme: "http", Host: host, Port: port})
	if !got.Ready || got.Detail != "HTTP 308" {
		t.Fatalf("redirect probe = %#v", got)
	}

	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failed.Close()
	host, port = hostPort(t, failed.URL)
	got = ProbeHTTP(context.Background(), Endpoint{Service: "web", Scheme: "http", Host: host, Port: port})
	if got.Ready || got.Detail != "HTTP 503" {
		t.Fatalf("5xx probe = %#v", got)
	}
}

func TestProbeHTTPDialTargetKeepsLogicalHost(t *testing.T) {
	var requestHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	dialHost, dialPort := hostPort(t, server.URL)
	logical := Endpoint{Service: "edge", Scheme: "http", Host: "mail.example.test", Port: dialPort}
	got := ProbeHTTPDialTarget(context.Background(), logical, dialHost, dialPort)
	if !got.Ready {
		t.Fatalf("probe = %#v", got)
	}
	if requestHost != "mail.example.test:"+strconv.Itoa(dialPort) {
		t.Fatalf("request host = %q", requestHost)
	}
}

func hostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
