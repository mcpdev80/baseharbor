package machinehttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

type testTerminal struct {
	reader        *io.PipeReader
	writer        *io.PipeWriter
	closed        chan struct{}
	reading       chan struct{}
	readOnce      sync.Once
	closeOnce     sync.Once
	mu            sync.Mutex
	input         []byte
	rows, columns int
	exit          int
	finished      chan struct{}
}

func newTestTerminal() *testTerminal {
	r, w := io.Pipe()
	return &testTerminal{reader: r, writer: w, closed: make(chan struct{}), reading: make(chan struct{}), finished: make(chan struct{})}
}
func (s *testTerminal) Read(data []byte) (int, error) {
	s.readOnce.Do(func() { close(s.reading) })
	return s.reader.Read(data)
}
func (s *testTerminal) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input = append(s.input, data...)
	return len(data), nil
}
func (s *testTerminal) Resize(rows, columns int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows, s.columns = rows, columns
	return nil
}
func (s *testTerminal) Close() error {
	s.closeOnce.Do(func() { _ = s.reader.Close(); _ = s.writer.Close(); close(s.closed) })
	return nil
}
func (s *testTerminal) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.finished:
		return s.exit, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

type terminalTestExecutor struct {
	executorFunc
	session *testTerminal
}

func (e terminalTestExecutor) OpenTerminal(context.Context, machine.StreamRequest) (terminal.Session, error) {
	return e.session, nil
}

func (e terminalTestExecutor) RecordTerminalAudit(context.Context, machine.StreamDescriptor, string) error {
	return nil
}

const terminalBody = `{"contract_version":"v1","context":{"environment":"dev","target":"local"},"resource_kind":"container","resource_id":"owned","command":["cat"],"tty":true,"rows":24,"columns":80}`

func openTestTerminal(t *testing.T, expiry *time.Time) (*Handler, *testTerminal, string) {
	t.Helper()
	session := newTestTerminal()
	h, err := New(terminalTestExecutor{session: session})
	if err != nil {
		t.Fatal(err)
	}
	r := withTestPrincipal(httptest.NewRequest("POST", "https://core.example/api/v1/machine/terminals", strings.NewReader(terminalBody)))
	p, _ := identity.FromContext(r.Context())
	p.ExpiresAt = expiry
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("creation: %d %s", w.Code, w.Body.String())
	}
	var descriptor machine.StreamDescriptor
	if err := json.Unmarshal(w.Body.Bytes(), &descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.Actor.Mode != "authenticated" || descriptor.Actor.Subject != "operator-123" {
		t.Fatal("terminal actor not bound")
	}
	t.Cleanup(func() {
		if record := h.terminals.get(descriptor.StreamID); record != nil {
			record.cancel()
		}
	})
	return h, session, descriptor.StreamID
}

func terminalRequest(h *Handler, id, method, suffix, body string, foreign bool) *httptest.ResponseRecorder {
	r := withTestPrincipal(httptest.NewRequest(method, "https://core.example/api/v1/machine/terminals/"+id+suffix, strings.NewReader(body)))
	if foreign {
		p, _ := identity.FromContext(r.Context())
		p.Subject = "foreign-operator"
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestTerminalInputResizeExitAndNoSideEffectReplay(t *testing.T) {
	h, session, id := openTestTerminal(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		r := withTestPrincipal(httptest.NewRequest("GET", "https://core.example/api/v1/machine/terminals/"+id+"/events", nil))
		h.ServeHTTP(events, r.WithContext(identity.WithPrincipal(ctx, mustTestPrincipal(r))))
		close(done)
	}()
	select {
	case <-session.reading:
	case <-time.After(time.Second):
		t.Fatal("events never attached")
	}
	for _, request := range []struct {
		body   string
		status int
	}{
		{`{"contract_version":"v1","sequence":1,"kind":"input","data":"aGVsbG8="}`, 204},
		{`{"contract_version":"v1","sequence":1,"kind":"input","data":"aGVsbG8="}`, 409},
		{`{"contract_version":"v1","sequence":2,"kind":"resize","rows":30,"columns":100}`, 204},
	} {
		if w := terminalRequest(h, id, "POST", "/input", request.body, false); w.Code != request.status {
			t.Fatalf("input: %d %s", w.Code, w.Body.String())
		}
	}
	session.mu.Lock()
	got := string(session.input)
	rows, cols := session.rows, session.columns
	session.mu.Unlock()
	if got != "hello" || rows != 30 || cols != 100 {
		t.Fatal("replayed input or lost resize", got, rows, cols)
	}
	go func() {
		_, _ = session.writer.Write([]byte("output"))
		session.exit = 7
		close(session.finished)
		_ = session.writer.Close()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("terminal exit did not end events")
	}
	if !strings.Contains(events.Body.String(), `"kind":"terminal.output"`) || !strings.Contains(events.Body.String(), `"exit_code":7`) {
		t.Fatal("wire output/exit lost", events.Body.String())
	}
}

func mustTestPrincipal(r *http.Request) *identity.Principal {
	p, _ := identity.FromContext(r.Context())
	return p
}

func TestTerminalForeignActorExpiryDisconnectAndReplayFailClosed(t *testing.T) {
	expiry := time.Now().Add(150 * time.Millisecond)
	h, session, id := openTestTerminal(t, &expiry)
	for _, path := range []struct{ method, suffix string }{{"POST", "/input"}, {"GET", "/events"}, {"DELETE", ""}} {
		if w := terminalRequest(h, id, path.method, path.suffix, `{}`, true); w.Code != 403 {
			t.Fatal("foreign actor admitted", w.Code)
		}
	}
	r := withTestPrincipal(httptest.NewRequest("GET", "https://core.example/api/v1/machine/terminals/"+id+"/events", nil))
	r.Header.Set("Last-Event-ID", "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("terminal replay accepted")
	}
	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("expiry did not close unattached terminal")
	}

	h, session, id = openTestTerminal(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r := withTestPrincipal(httptest.NewRequest("GET", "https://core.example/api/v1/machine/terminals/"+id+"/events", nil))
		h.ServeHTTP(httptest.NewRecorder(), r.WithContext(identity.WithPrincipal(ctx, mustTestPrincipal(r))))
		close(done)
	}()
	select {
	case <-session.reading:
	case <-time.After(time.Second):
		t.Fatal("output not attached")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disconnect left idle reader running")
	}
	select {
	case <-session.closed:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not close session")
	}
}
