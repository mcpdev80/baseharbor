package controlplaneruntime

import (
	"errors"
	"testing"
)

func TestConfigValidateRequiresDatabaseURL(t *testing.T) {
	err := (Config{}).Validate()
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("Validate() error = %v, want ErrMissingDatabaseURL", err)
	}
}

func TestConfigValidateRequiresTLSFiles(t *testing.T) {
	err := (Config{DatabaseURL: "postgres://example"}).Validate()
	if !errors.Is(err, ErrMissingTLSFiles) {
		t.Fatalf("Validate() error = %v, want ErrMissingTLSFiles", err)
	}
}

func TestDefaultListenAddrIsLoopbackTLSPort(t *testing.T) {
	if got := (Config{}).listenAddr(); got != "127.0.0.1:8443" {
		t.Fatalf("listenAddr() = %q, want loopback default", got)
	}
}
