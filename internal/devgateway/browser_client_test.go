package devgateway

import (
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestGatewayBrowserClientUsesOwnedListenerWithoutDNS(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	listener, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = server.Listener.Close()
	server.Listener = listener
	server.StartTLS()
	defer server.Close()
	_, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	files, err := FilesFor("own")
	if err != nil {
		t.Fatal(err)
	}
	if err := saveState(files.State, state{Version: stateVersion, BindAddress: "127.0.0.2", HostPort: port, Routes: []Route{{Owner: "owned", Key: "owned/browser", Host: "example.com", Upstream: "http://app:8080", Network: "owned"}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(files.CA), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.CA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := BrowserClient("own", "example.com")
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	response, err := client.Get("https://example.com:" + strconv.Itoa(port) + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("owned listener not reached")
	}
	if _, err := BrowserClient("own", "foreign.example.com"); err == nil {
		t.Fatal("unregistered authority accepted")
	}
	if _, err := client.Get("https://other.example.com:" + strconv.Itoa(port) + "/"); err == nil {
		t.Fatal("foreign destination was dialed")
	}
}
