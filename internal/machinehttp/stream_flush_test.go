package machinehttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type controlledLogExecutor struct {
	executorFunc
	reader *io.PipeReader
}

func (e controlledLogExecutor) OpenLogStream(context.Context, machine.StreamRequest) (io.ReadCloser, error) {
	return e.reader, nil
}

func TestHTTPSLogStreamAdmitsIdleProducerAndFlushesSmallOutput(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	handler, err := New(controlledLogExecutor{reader: reader})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, withTestPrincipal(r))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	body := `{"contract_version":"v1","context":{"environment":"dev","target":"owned-target"},"resource_kind":"container","resource_id":"owned","follow":true}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/machine/streams/logs", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("idle log producer withheld admission headers: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("X-BaseHarbor-Resource-ID") != "owned" {
		t.Fatal("log admission differs")
	}
	go func() { _, _ = writer.Write([]byte("live\n")) }()
	data := make([]byte, 5)
	if _, err := io.ReadFull(response.Body, data); err != nil || string(data) != "live\n" {
		t.Fatalf("small live output remained buffered: %q, %v", data, err)
	}
}
