package serviceaccess

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// TestPersesGatewayReachabilityAuthAndHostInvisibility is the isolated
// integration test for the Perses management surface. A listener plays the
// Perses server bound exactly the way the rendering binds it — loopback of
// the shared network namespace, never a published port — and the genuine
// gateway material (EnsureHTTPGateway, mTLS policy) fronts it through a TLS
// server that enforces client certificates and refuses the datasource-proxy
// prefix the same way the Caddy rendering does. It proves: the gateway
// reaches the upstream through the shared namespace; anonymous access is
// denied; the datasource proxy route is refused; and Perses itself never
// becomes host-visible.
func TestPersesGatewayReachabilityAuthAndHostInvisibility(t *testing.T) {
	plan, err := BuildPersesGateway(persesRequirements(AuthenticationMTLS))
	if err != nil {
		t.Fatalf("build the mTLS plan: %v", err)
	}
	if !plan.SharedGatewayNetwork || plan.UpstreamBindAddress != "127.0.0.1" {
		t.Fatalf("the plan must render the shared-namespace loopback topology, got %+v", plan)
	}
	registerPersesRendererForTest(t, plan)

	upstreamPort, upstreamServes := startPersesUpstream(t, plan)
	front := startPersesGatewayFront(t, t.TempDir(), plan, upstreamPort)

	t.Run("gateway reaches the upstream through the shared namespace", func(t *testing.T) {
		response, err := front.client.Get(front.url + "/api/v1/health")
		if err != nil {
			t.Fatalf("the gateway must reach Perses on the shared loopback: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("health through the gateway = %d, want 200", response.StatusCode)
		}
		if !*upstreamServes {
			t.Fatal("the upstream never served the proxied request")
		}
	})

	t.Run("authentication is enforced at the gateway", func(t *testing.T) {
		response, err := front.anonymous.Get(front.url + "/api/v1/health")
		if err != nil {
			return // the TLS handshake itself refuses the anonymous caller
		}
		defer response.Body.Close()
		t.Fatalf("an anonymous caller without a client certificate must never reach the API, got %d", response.StatusCode)
	})

	t.Run("the datasource proxy route is refused", func(t *testing.T) {
		response, err := front.client.Get(front.url + capability.PersesProxyPrefix + "global/datasource/1")
		if err != nil {
			t.Fatalf("the proxy request must be answered, not fail in transport: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("the datasource proxy prefix must be refused with 404, got %d", response.StatusCode)
		}
	})

	t.Run("Perses is not directly visible on the host", func(t *testing.T) {
		rendered, err := PersesComposeServices(context.Background(), plan, front.files, capability.PersesDefaultRelease)
		if err != nil {
			t.Fatalf("the services must render: %v", err)
		}
		persesBlock := rendered
		if idx := strings.Index(rendered, "  "+PersesServiceName+":"); idx >= 0 {
			persesBlock = rendered[idx:]
		}
		if strings.Contains(persesBlock, "ports:") {
			t.Fatalf("the Perses service must never be published to the host:\n%s", persesBlock)
		}
		if !strings.Contains(persesBlock, `network_mode: "service:`+PersesGatewayServiceName+`"`) {
			t.Fatalf("the Perses service must join the gateway namespace so its loopback is reachable yet unforwardable:\n%s", persesBlock)
		}
		if strings.Contains(persesBlock, "expose:") {
			t.Fatalf("the Perses service must not expose ports even implicitly:\n%s", persesBlock)
		}
		if !strings.Contains(rendered, `127.0.0.1:${`+PersesHostPortEnv) {
			t.Fatalf("the gateway must be the only published endpoint, on loopback:\n%s", rendered)
		}
	})
}

// persesGatewayFront is the TLS-terminated gateway endpoint the operator
// talks to, with verified and verified-free HTTP clients for the tests.
type persesGatewayFront struct {
	url       string
	client    *http.Client
	anonymous *http.Client
	files     HTTPGatewayFiles
}

func startPersesUpstream(t *testing.T, plan PersesGatewayPlan) (int, *bool) {
	t.Helper()
	served := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		served = true
		_, _ = w.Write([]byte(`{"healthy":true}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		served = true
		_, _ = w.Write([]byte("perses"))
	})
	server := httptest.NewUnstartedServer(mux)
	// The upstream binds the SAME kind of address the rendered service
	// definition binds it to: loopback of this process' namespace, exactly the
	// loopback of the namespace the gateway joins in the shared form.
	if plan.UpstreamBindAddress != "127.0.0.1" {
		t.Fatalf("the plan must bind loopback, got %q", plan.UpstreamBindAddress)
	}
	binding := &loopbackListener{t: t}
	binding.bind()
	server.Listener = binding
	server.Start()
	t.Cleanup(server.Close)
	return binding.port(), &served
}

// loopbackListener binds the loopback of this network namespace exactly the
// way the Perses container binds it inside the namespace the gateway joins.
type loopbackListener struct {
	t *testing.T
	l net.Listener
}

func (p *loopbackListener) bind() {
	p.t.Helper()
	if p.l != nil {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		p.t.Fatalf("bind the upstream on the shared-namespace loopback: %v", err)
	}
	p.l = listener
}

func (p *loopbackListener) port() int {
	return p.l.Addr().(*net.TCPAddr).Port
}

func (p *loopbackListener) Accept() (net.Conn, error) {
	p.bind()
	return p.l.Accept()
}

func (p *loopbackListener) Close() error {
	if p.l != nil {
		return p.l.Close()
	}
	return nil
}

func (p *loopbackListener) Addr() net.Addr {
	p.bind()
	return p.l.Addr()
}

func startPersesGatewayFront(t *testing.T, stateDir string, plan PersesGatewayPlan, upstreamPort int) persesGatewayFront {
	t.Helper()
	ctx := context.Background()
	spec := plan.Spec
	// The gateway upstream is the Perses SERVICE ADDRESS, and the frontend
	// resolves it exactly like compose DNS inside the shared namespace: to
	// the loopback the Perses stand-in bound. That is the namespace-local
	// name->address step the rendering's network_mode delegates to.
	spec.Upstream = "http://" + PersesServiceName + ":" + strconv.Itoa(upstreamPort)
	spec.Upstreams = nil
	policy, err := Resolve("development", PersesGatewayProvider, AuthenticationMTLS)
	if err != nil {
		t.Fatalf("resolve the managed gateway policy: %v", err)
	}
	// The gateway authority verifies operator certificates against its own
	// trust: the operator identity must be a leaf of the SAME CA that
	// materialized the gateway, never of a second test CA.
	issuer := newTestIssuer(t)
	files, err := EnsureHTTPGateway(ctx, issuer, policy, stateDir, spec)
	if err != nil {
		t.Fatalf("materialize the gateway: %v", err)
	}
	if _, err := os.Stat(files.Caddyfile); err != nil {
		t.Fatalf("the gateway must materialize its Caddyfile: %v", err)
	}
	serverPair, err := tls.LoadX509KeyPair(files.Material.ServerCertificate, files.Material.ServerKey)
	if err != nil {
		t.Fatalf("load the projected server pair: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(mustReadPEM(t, files.Material.CA)) {
		t.Fatalf("the projected trust bundle is not loadable")
	}
	// The gateway authority verifies operator certificates against its own
	// trust: the operator identity is a leaf of the SAME authority that
	// materialized the gateway, never of a second CA.
	operatorMaterial := persesOperatorIdentityFromGateway(t, issuer, stateDir)
	clientPair, err := tls.LoadX509KeyPair(operatorMaterial.ServerCertificate, operatorMaterial.ServerKey)
	if err != nil {
		t.Fatalf("load the operator identity pair: %v", err)
	}
	denied := plan.DeniedPaths
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		for _, prefix := range denied {
			if strings.HasPrefix(request.URL.Path, strings.TrimSpace(prefix)) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
		target := spec.Upstream + request.URL.Path
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
			// The shared-namespace DNS: the Perses service address resolves
			// to the loopback the Perses container bound. Outside the
			// shared namespace the name resolves nowhere and the request
			// fails closed — exactly the property under test.
			DialContext: persesServiceResolver(upstreamPort),
		}}
		upstream, err := client.Get(target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstream.Body.Close()
		w.WriteHeader(upstream.StatusCode)
		_, _ = io.Copy(w, upstream.Body)
	})
	front := httptest.NewUnstartedServer(handler)
	front.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverPair},
		ClientAuth:   tls.RequestClientCert,
		// The managed trust issues service leaves without a clientAuth EKU,
		// so the stdlib chain verify is skipped in favour of an explicit one
		// against the projected gateway trust; an absent certificate and the
		// server's own pair can never impersonate an operator.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("no client certificate presented")
			}
			leaf, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			if _, verifyErr := leaf.Verify(x509.VerifyOptions{
				Roots:         roots,
				CurrentTime:   time.Now(),
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
				Intermediates: x509.NewCertPool(),
			}); verifyErr != nil {
				return errors.New("the operator certificate is not signed by the gateway trust: " + verifyErr.Error())
			}
			if bytes.Equal(leaf.Raw, serverPair.Certificate[0]) {
				return errors.New("the server pair is not an operator identity")
			}
			return nil
		},
	}
	front.StartTLS()
	t.Cleanup(front.Close)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientPair},
	}}}
	anonymous := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}}}
	t.Cleanup(func() {
		client.Transport.(*http.Transport).CloseIdleConnections()
		anonymous.Transport.(*http.Transport).CloseIdleConnections()
	})
	return persesGatewayFront{url: front.URL, client: client, anonymous: anonymous, files: files}
}

func mustReadPEM(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// persesOperatorIdentityFromGateway issues an operator client leaf from the
// SAME authority that materialized the gateway (shared issuer instance),
// reproducing the exact trust relationship EnsureHTTPGateway established.
func persesOperatorIdentityFromGateway(t *testing.T, issuer *testIssuer, stateDir string) TLSMaterial {
	t.Helper()
	material, err := EnsureTLSMaterial(context.Background(), issuer, mustResolveMTLSPolicy(t), filepath.Join(stateDir, "operator-pki"), "perses-operator")
	if err != nil {
		t.Fatalf("issue the operator identity: %v", err)
	}
	return material
}

func mustResolveMTLSPolicy(t *testing.T) Policy {
	t.Helper()
	policy, err := Resolve("development", PersesGatewayProvider, AuthenticationMTLS)
	if err != nil {
		t.Fatalf("resolve the mTLS policy: %v", err)
	}
	return policy
}

// persesServiceResolver stands in for compose DNS inside the shared network
// namespace: the Perses service name resolves to the loopback address the
// Perses container bound. Any other name fails closed, never to an arbitrary
// host route.
func persesServiceResolver(port int) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if host != PersesServiceName {
			return nil, fmt.Errorf("compose DNS in the shared namespace does not resolve %q", host)
		}
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		return dialer.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(port))
	}
}
