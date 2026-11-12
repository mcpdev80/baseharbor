package devgateway

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestGatewayVerificationUsesRecordedBindAndRealmPath(t *testing.T) {
	called := false
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/dev/console/" {
			http.Error(w, "wrong realm landing", 500)
			return
		}
		called = true
		w.WriteHeader(200)
	}))
	listener, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = server.Listener.Close()
	server.Listener = listener
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	_, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	route := Route{Host: "example.com", LandingPath: "/admin/dev/console/"}
	if err := verifyRouteBound(context.Background(), roots, route, port, "127.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("recorded LAN binding was not reached")
	}
}

func TestGatewayDirectIPVerificationFollowsOwnedRedirectPort(t *testing.T) {
	reached := false
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true; w.WriteHeader(200) }))
	defer destination.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/start" {
			http.Error(w, "wrong listener", 500)
			return
		}
		http.Redirect(w, r, destination.URL+"/finish", 302)
	}))
	defer source.Close()
	port := func(raw string) int { u, _ := url.Parse(raw); n, _ := strconv.Atoi(u.Port()); return n }
	roots := x509.NewCertPool()
	roots.AddCert(source.Certificate())
	roots.AddCert(destination.Certificate())
	route := Route{Owner: "shared/identity", ManagementKey: "identity", DirectIPPort: port(source.URL), LandingPath: "/start"}
	current := state{IPAccessAddress: "127.0.0.1", Routes: []Route{route, {Owner: route.Owner, DirectIPPort: port(destination.URL)}}}
	if err := verifyDirectIPRoute(context.Background(), roots, current, route); err != nil {
		t.Fatal(err)
	}
	if !reached {
		t.Fatal("redirect was incorrectly sent to the original port")
	}
}
