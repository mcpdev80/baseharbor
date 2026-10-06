package targetsession

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

type registry struct {
	scope  targetenrollment.Scope
	denied atomic.Bool
	calls  atomic.Int32
}

func (r *registry) AdmitCertificate(_ context.Context, scope targetenrollment.Scope, _ string, _ time.Time) error {
	r.calls.Add(1)
	if r.denied.Load() || scope != r.scope {
		return targetenrollment.ErrDenied
	}
	return nil
}

func testNode() Node {
	n := Node{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "lab", NodeID: "node-a", Runtime: "docker"}
	n.Identity = n.Scope().Identity()
	return n
}

func newSessionPair(t *testing.T, denied bool) (*Session, *tls.Conn, *registry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	node := testNode()
	issuer := serviceissuer.New(t)
	uri, _ := url.Parse(node.Identity)
	clientMaterial, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: node.Identity, URIs: []*url.URL{uri}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	coreURI, _ := url.Parse("spiffe://baseharbor/platform/core/test")
	serverMaterial, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: "core.example", DNSNames: []string{"core.example"}, URIs: []*url.URL{coreURI}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	clientPair, err := tls.X509KeyPair(clientMaterial.Certificate, clientMaterial.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPair, err := tls.X509KeyPair(serverMaterial.Certificate, serverMaterial.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	trust, _ := issuer.TrustBundle(ctx)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(trust.PEM)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	client := tls.Client(left, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: "core.example", Certificates: []tls.Certificate{clientPair}})
	server := tls.Server(right, &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{serverPair}})
	r := &registry{scope: node.Scope()}
	r.denied.Store(denied)
	type result struct {
		session *Session
		err     error
	}
	accepted := make(chan result, 1)
	go func() { s, e := Accept(ctx, server, r, coreURI.String()); accepted <- result{s, e} }()
	if err := client.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := writeRecord(client, "hello", hello{[]string{contractVersion}, []string{protocolVersion}, node}); err != nil {
		t.Fatal(err)
	}
	if denied {
		out := <-accepted
		if out.err == nil || out.session != nil {
			t.Fatal("revoked identity admitted")
		}
		return nil, client, r
	}
	var greeting hello
	if err := readRecord(client, "hello", &greeting); err != nil {
		t.Fatal(err)
	}
	if greeting.Node.Identity != coreURI.String() {
		t.Fatal("Core identity not bound")
	}
	var request Request
	if err := readRecord(client, "request", &request); err != nil {
		t.Fatal(err)
	}
	if string(request.Payload) != "{}" {
		t.Fatalf("capability negotiation requires a canonical empty object, got %s", request.Payload)
	}
	capabilities := Capabilities{contractVersion, protocolVersion, node, []Capability{{Name: "runtime.detect", Available: true}}}
	data, _ := json.Marshal(capabilities)
	if err := writeRecord(client, "response", reply(request, data)); err != nil {
		t.Fatal(err)
	}
	out := <-accepted
	if out.err != nil {
		t.Fatal(out.err)
	}
	t.Cleanup(func() { out.session.Close() })
	return out.session, client, r
}

func requestNow(id string) Request {
	now := time.Now().UTC()
	return Request{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, RequestID: id, CorrelationID: "execution-a", TargetID: "lab", Operation: "runtime.detect", IssuedAt: now, DeadlineAt: now.Add(time.Second)}
}
func reply(request Request, data json.RawMessage) Response {
	return Response{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, RequestID: request.RequestID, CorrelationID: request.CorrelationID, Success: true, Result: data}
}

func TestVerifiedSessionBindsNegotiatedCapabilitiesAndCorrelation(t *testing.T) {
	s, client, r := newSessionPair(t, false)
	caps, err := s.LiveCapabilities()
	if err != nil || caps.Node != testNode() {
		t.Fatal(caps, err)
	}
	caps.Capabilities[0].Available = false
	caps, _ = s.LiveCapabilities()
	if !caps.Capabilities[0].Available {
		t.Fatal("capability copy mutated session")
	}
	go func() {
		var request Request
		if readRecord(client, "request", &request) == nil {
			_ = writeRecord(client, "response", reply(request, json.RawMessage(`{"runtime":"docker"}`)))
		}
	}()
	response, err := s.Dispatch(context.Background(), requestNow("request-a"))
	if err != nil || !response.Success || r.calls.Load() < 3 {
		t.Fatal("authenticated dispatch failed", response, err)
	}
}

func TestInvalidSelectionCannotWriteAndRevocationClosesSession(t *testing.T) {
	s, _, r := newSessionPair(t, false)
	for _, kind := range []string{"target", "operation", "expiry", "future", "correlation"} {
		request := requestNow("invalid-a")
		switch kind {
		case "target":
			request.TargetID = "foreign"
		case "operation":
			request.Operation = "runtime_command"
		case "expiry":
			request.DeadlineAt = time.Now().Add(-time.Second)
		case "future":
			request.IssuedAt = time.Now().UTC().Add(time.Hour)
		case "correlation":
			request.CorrelationID = ""
		}
		if _, err := s.Dispatch(context.Background(), request); !errors.Is(err, contracts.ErrTargetAccessWire) {
			t.Fatal(kind, err)
		}
	}
	r.denied.Store(true)
	if _, err := s.Dispatch(context.Background(), requestNow("revoked-a")); !errors.Is(err, ErrUnavailable) || s.ctx.Err() == nil {
		t.Fatal("revoked session remained live", err)
	}
}

func TestInactiveCAIssuedCertificateCannotEnrollSession(t *testing.T) { newSessionPair(t, true) }

func TestIdleSessionRevocationStopsLiveCapabilitiesWithinCheckBound(t *testing.T) {
	s, _, r := newSessionPair(t, false)
	r.denied.Store(true)
	select {
	case <-s.ctx.Done():
		if _, err := s.LiveCapabilities(); !errors.Is(err, ErrUnavailable) {
			t.Fatal("revoked idle session advertised capability", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("idle revocation exceeded admission check bound")
	}
}

func TestDisconnectAndForeignResponsesRetireWithoutReplay(t *testing.T) {
	for _, mode := range []string{"disconnect", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			s, client, _ := newSessionPair(t, false)
			go func() {
				var request Request
				if readRecord(client, "request", &request) != nil {
					return
				}
				if mode == "disconnect" {
					client.Close()
					return
				}
				request.CorrelationID = "foreign"
				_ = writeRecord(client, "response", reply(request, json.RawMessage(`{}`)))
			}()
			if _, err := s.Dispatch(context.Background(), requestNow("request-a")); !errors.Is(err, ErrUnavailable) || s.ctx.Err() == nil {
				t.Fatal("ambiguous operation retained session", err)
			}
			if _, err := s.Dispatch(context.Background(), requestNow("request-b")); !errors.Is(err, ErrUnavailable) {
				t.Fatal("retired connection reused", err)
			}
		})
	}
}

func TestCancellationClosesBlockedResponse(t *testing.T) {
	s, client, _ := newSessionPair(t, false)
	read := make(chan struct{})
	go func() { var request Request; _ = readRecord(client, "request", &request); close(read) }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := s.Dispatch(ctx, requestNow("request-a")); done <- err }()
	<-read
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation left dispatch blocked")
	}
}

func TestFramesRejectOversizeDuplicateAndTruncatedRecords(t *testing.T) {
	for _, data := range [][]byte{[]byte(`{"contract_version":"x","contract_version":"y"}`), {1, 2}} {
		var buf bytes.Buffer
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(data)))
		buf.Write(header[:])
		buf.Write(data)
		var response Response
		if readRecord(&buf, "response", &response) == nil {
			t.Fatal("malformed frame accepted")
		}
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], contracts.TargetAccessMaxFrameBytes+1)
	var response Response
	if !errors.Is(readRecord(bytes.NewReader(header[:]), "response", &response), contracts.ErrTargetAccessWire) {
		t.Fatal("oversize frame allocated")
	}
	if !errors.Is(readRecord(bytes.NewReader([]byte{0, 0}), "response", &response), io.ErrUnexpectedEOF) {
		t.Fatal("truncated header accepted")
	}
}

func TestPoolNeverSelectsAnotherTenantOrNode(t *testing.T) {
	s, _, _ := newSessionPair(t, false)
	p := NewPool()
	if p.add(s) != nil {
		t.Fatal("session not added")
	}
	for _, kind := range []string{"tenant", "node", "runtime"} {
		scope := s.Scope()
		switch kind {
		case "tenant":
			scope.TenantID = "22222222-2222-4222-8222-222222222222"
		case "node":
			scope.NodeID = "node-b"
		case "runtime":
			scope.Runtime = "podman"
		}
		if _, err := p.Dispatch(context.Background(), scope, requestNow("request-a")); !errors.Is(err, ErrUnavailable) {
			t.Fatal("foreign selection admitted", kind, err)
		}
	}
}
