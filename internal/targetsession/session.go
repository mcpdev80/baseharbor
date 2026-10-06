package targetsession

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

var ErrUnavailable = errors.New("authenticated Connector session is unavailable; reconcile before retrying a mutation")

// Session has one bounded control request in flight. Failed or cancelled
// dispatch always retires the connection; a side effect is never auto-replayed.
type Session struct {
	conn         *tls.Conn
	registry     targetenrollment.NodeRegistry
	node         Node
	ctx          context.Context
	cancel       context.CancelFunc
	busy         chan struct{}
	writeMu      sync.Mutex
	once         sync.Once
	capabilities Capabilities
}

// Accept exchanges the canonical Hello only after a verified TLS 1.3 handshake.
// The caller configures RequireAndVerifyClientCert and a trusted Core server
// certificate. coreIdentity must be that certificate's sole SPIFFE URI.
func Accept(ctx context.Context, conn *tls.Conn, registry targetenrollment.NodeRegistry, coreIdentity string) (*Session, error) {
	if conn == nil || registry == nil || !strings.HasPrefix(coreIdentity, "spiffe://baseharbor/platform/core/") {
		return nil, ErrUnavailable
	}
	success := false
	defer func() {
		if !success {
			_ = conn.NetConn().Close()
		}
	}()
	stopHandshake := context.AfterFunc(ctx, func() { _ = conn.NetConn().Close() })
	defer stopHandshake()
	handshake, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	deadline, _ := handshake.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, ErrUnavailable
	}
	if err := conn.HandshakeContext(handshake); err != nil {
		return nil, ErrUnavailable
	}
	var remote hello
	if err := readRecord(conn, "hello", &remote); err != nil {
		return nil, ErrUnavailable
	}
	if remote.Node.Identity != remote.Node.Scope().Identity() || targetenrollment.AdmitTLSNode(handshake, conn.ConnectionState(), remote.Node.Scope(), registry) != nil {
		return nil, ErrUnavailable
	}
	local := hello{ContractVersions: []string{contractVersion}, ProtocolVersions: []string{protocolVersion}, Node: remote.Node}
	local.Node.NodeID = "core"
	local.Node.Identity = coreIdentity
	local.Node.InstanceID = ""
	if err := writeRecord(conn, "hello", local); err != nil {
		return nil, ErrUnavailable
	}
	expires := time.Now().Add(5 * time.Minute)
	leaf := conn.ConnectionState().PeerCertificates[0]
	if leaf.NotAfter.Before(expires) {
		expires = leaf.NotAfter
	}
	lifetime, stop := context.WithDeadline(ctx, expires)
	s := &Session{conn: conn, registry: registry, node: remote.Node, ctx: lifetime, cancel: stop, busy: make(chan struct{}, 1)}
	if err := conn.SetDeadline(expires); err != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	go s.watchAdmission()
	now := time.Now().UTC()
	response, err := s.Dispatch(lifetime, Request{ContractVersion: contractVersion, ProtocolVersion: protocolVersion, RequestID: "admission-capabilities", CorrelationID: "admission-capabilities", TargetID: remote.Node.TargetID, Operation: "connector.capabilities", IssuedAt: now, DeadlineAt: now.Add(10 * time.Second)})
	if err != nil || !response.Success || contracts.ValidateTargetAccessRecord("capabilities", response.Result) != nil {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	if json.Unmarshal(response.Result, &s.capabilities) != nil || s.capabilities.Node != s.node {
		_ = s.Close()
		return nil, ErrUnavailable
	}
	success = true
	return s, nil
}

func (s *Session) watchAdmission() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer s.Close()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			check, cancel := context.WithTimeout(s.ctx, 2*time.Second)
			err := targetenrollment.AdmitTLSNode(check, s.conn.ConnectionState(), s.node.Scope(), s.registry)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		s.cancel()
		// Retiring an interrupted operation must unblock reads and writes before
		// TLS close_notify. A slow peer cannot extend the cancellation bound.
		_ = s.conn.SetDeadline(time.Now())
		// TLS Close resets its close_notify deadline to five seconds. Abort the
		// underlying transport instead: this session is deliberately retired.
		err = s.conn.NetConn().Close()
	})
	return err
}

func (s *Session) Scope() targetenrollment.Scope { return s.node.Scope() }

// LiveCapabilities returns a copy of the authenticated negotiation, never the
// provider descriptor. Unavailable sessions advertise no live capability.
func (s *Session) LiveCapabilities() (Capabilities, error) {
	if s.ctx.Err() != nil {
		return Capabilities{}, ErrUnavailable
	}
	c := s.capabilities
	c.Capabilities = append([]Capability(nil), c.Capabilities...)
	return c, nil
}

func (s *Session) Dispatch(ctx context.Context, request Request) (Response, error) {
	if s == nil || ctx.Err() != nil || s.ctx.Err() != nil {
		return Response{}, ErrUnavailable
	}
	select {
	case s.busy <- struct{}{}:
		defer func() { <-s.busy }()
	default:
		return Response{}, ErrUnavailable
	}
	data, err := json.Marshal(request)
	now := time.Now().UTC()
	if err != nil || contracts.ValidateTargetAccessRecord("request", data) != nil || request.TargetID != s.node.TargetID ||
		!request.DeadlineAt.After(now) || !request.DeadlineAt.After(request.IssuedAt) || request.DeadlineAt.After(now.Add(30*time.Minute)) ||
		request.IssuedAt.Before(now.Add(-5*time.Minute)) || request.IssuedAt.After(now.Add(5*time.Minute)) {
		return Response{}, contracts.ErrTargetAccessWire
	}
	if s.capabilities.Node.NodeID != "" && request.Operation != "connector.capabilities" {
		available := false
		for _, capability := range s.capabilities.Capabilities {
			if capability.Name == request.Operation && capability.Available {
				available = true
			}
		}
		if !available {
			return Response{}, ErrUnavailable
		}
	}
	check, stop := context.WithTimeout(ctx, 2*time.Second)
	err = targetenrollment.AdmitTLSNode(check, s.conn.ConnectionState(), s.node.Scope(), s.registry)
	stop()
	if err != nil {
		_ = s.Close()
		return Response{}, ErrUnavailable
	}
	operation, cancel := context.WithDeadline(ctx, request.DeadlineAt)
	defer cancel()
	stopSession := context.AfterFunc(s.ctx, cancel)
	defer stopSession()
	deadline, _ := operation.Deadline()
	if err := s.conn.SetReadDeadline(deadline); err != nil {
		_ = s.Close()
		return Response{}, ErrUnavailable
	}
	stopCancellation := context.AfterFunc(operation, func() { _ = s.Close() })
	defer stopCancellation()
	s.writeMu.Lock()
	writeDeadline := time.Now().Add(5 * time.Second)
	if deadline.Before(writeDeadline) {
		writeDeadline = deadline
	}
	err = s.conn.SetWriteDeadline(writeDeadline)
	if err == nil {
		err = writeRecord(s.conn, "request", request)
	}
	s.writeMu.Unlock()
	if err != nil {
		_ = s.Close()
		return Response{}, ErrUnavailable
	}
	var response Response
	err = readRecord(s.conn, "response", &response)
	if err != nil || operation.Err() != nil || s.ctx.Err() != nil || response.RequestID != request.RequestID || response.CorrelationID != request.CorrelationID {
		_ = s.Close()
		return Response{}, ErrUnavailable
	}
	return response, nil
}
