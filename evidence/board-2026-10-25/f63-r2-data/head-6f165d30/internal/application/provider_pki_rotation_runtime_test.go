package application

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"
)

func assertOldCARootRejected(t *testing.T, caPEM []byte, port string) {
	t.Helper()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("old CA test bundle is invalid")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(loopbackHost, port), &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: loopbackHost,
	})
	if err == nil {
		_ = conn.Close()
		t.Fatal("retired CA still validates the rotated TLS endpoint")
	}
}
