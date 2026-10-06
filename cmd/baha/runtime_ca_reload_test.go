package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestOpenBaoCARetirementRequiresReloadedListener(t *testing.T) {
	oldPEM, oldKey := testCertificatePair(t, "openbao")
	newPEM, newKey := testCertificatePair(t, "openbao")
	oldCert, err := tls.X509KeyPair(oldPEM, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	newCert, err := tls.X509KeyPair(newPEM, newKey)
	if err != nil {
		t.Fatal(err)
	}
	var active atomic.Pointer[tls.Certificate]
	active.Store(&oldCert)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return active.Load(), nil }}
	server.StartTLS()
	defer server.Close()
	bundle := append(append([]byte{}, oldPEM...), newPEM...)
	path := filepath.Join(t.TempDir(), "overlap.pem")
	if err := os.WriteFile(path, bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	material := serviceaccess.TLSMaterial{CA: path, ServerName: "openbao"}
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	if err := serviceaccess.WaitHTTPS(context.Background(), client, server.URL, "/v1/sys/health"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := waitForOpenBaoReplacementCA(ctx, material, server.URL, newPEM); err == nil {
		t.Fatal("old listener was accepted before reload")
	}
	active.Store(&newCert)
	if err := waitForOpenBaoReplacementCA(context.Background(), material, server.URL, newPEM); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(path)
	if err != nil || string(retained) != string(bundle) {
		t.Fatal("verification changed overlap trust")
	}
}
