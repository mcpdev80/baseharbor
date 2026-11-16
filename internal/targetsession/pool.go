package targetsession

import (
	"context"
	"crypto/tls"
	"net"
	"sync"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

const maxConnections = 64
const maxScopeSessions = 4

// Pool is indexed by the exact enrolled scope. It never retries an invocation
// on a different connection and never resolves an unavailable remote locally.
type Pool struct {
	mu       sync.Mutex
	sessions map[targetenrollment.Scope][]*Session
}

func NewPool() *Pool { return &Pool{sessions: make(map[targetenrollment.Scope][]*Session)} }

func (p *Pool) LiveCapabilities(scope targetenrollment.Scope) (Capabilities, error) {
	if p == nil || scope.Validate() != nil {
		return Capabilities{}, ErrUnavailable
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, session := range p.sessions[scope] {
		if session.ctx.Err() == nil {
			return session.LiveCapabilities()
		}
	}
	return Capabilities{}, ErrUnavailable
}

func (p *Pool) add(s *Session) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := s.Scope()
	live := p.sessions[key][:0]
	for _, session := range p.sessions[key] {
		if session.ctx.Err() == nil {
			live = append(live, session)
		}
	}
	if len(live) >= maxScopeSessions {
		return ErrUnavailable
	}
	p.sessions[key] = append(live, s)
	return nil
}

func (p *Pool) remove(s *Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := s.Scope()
	var live []*Session
	for _, session := range p.sessions[key] {
		if session != s && session.ctx.Err() == nil {
			live = append(live, session)
		}
	}
	if len(live) == 0 {
		delete(p.sessions, key)
	} else {
		p.sessions[key] = live
	}
}

func (p *Pool) Dispatch(ctx context.Context, scope targetenrollment.Scope, request Request) (Response, error) {
	if p == nil || scope.Validate() != nil || request.TargetID != scope.TargetID {
		return Response{}, ErrUnavailable
	}
	p.mu.Lock()
	var selected *Session
	for _, session := range p.sessions[scope] {
		if session.ctx.Err() == nil && len(session.busy) == 0 {
			selected = session
			break
		}
	}
	p.mu.Unlock()
	if selected == nil {
		return Response{}, ErrUnavailable
	}
	return selected.Dispatch(ctx, request)
}

// Serve admits outbound connections with bounded handshake concurrency and
// per-scope capacity. Closing the listener or cancelling ctx retires sessions.
// Configuration and Core identity are validated by the TLS authority caller.
func (p *Pool) Serve(ctx context.Context, listener net.Listener, configuration *tls.Config, registry targetenrollment.NodeRegistry, coreIdentity string) error {
	if p == nil || listener == nil || configuration == nil || configuration.MinVersion < tls.VersionTLS13 ||
		configuration.ClientAuth != tls.RequireAndVerifyClientCert || registry == nil {
		return ErrUnavailable
	}
	lifetime, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(lifetime, func() { _ = listener.Close() })
	defer stop()
	slots := make(chan struct{}, maxConnections)
	var workers sync.WaitGroup
	defer workers.Wait()
	defer cancel()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = connection.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			secured := tls.Server(connection, configuration.Clone())
			stopConnection := context.AfterFunc(lifetime, func() { _ = connection.Close() })
			defer stopConnection()
			session, err := Accept(lifetime, secured, registry, coreIdentity)
			if err != nil {
				return
			}
			defer session.Close()
			if p.add(session) != nil {
				return
			}
			defer p.remove(session)
			<-session.ctx.Done()
		}()
	}
}
