package controlplaneruntime

import (
	"errors"
	"testing"
)

func TestConfigValidateRequiresTLSFiles(t *testing.T) {
	err := (Config{}).Validate()
	if !errors.Is(err, ErrMissingTLSFiles) {
		t.Fatalf("Validate() error = %v, want ErrMissingTLSFiles", err)
	}
}

func TestOperatorAPIIsOptIn(t *testing.T) {
	if (Config{}).operatorAPIEnabled() {
		t.Fatal("runtime-only config unexpectedly enables operator API")
	}
	if !(Config{OIDCIssuer: "https://issuer.example"}).operatorAPIEnabled() {
		t.Fatal("OIDC issuer should enable operator API validation")
	}
	if !(Config{OIDCAudiences: []string{"baseharbor"}}).operatorAPIEnabled() {
		t.Fatal("OIDC audience should enable operator API validation")
	}
}

func TestDefaultListenAddrIsLoopbackTLSPort(t *testing.T) {
	if got := (Config{}).listenAddr(); got != "127.0.0.1:8443" {
		t.Fatalf("listenAddr() = %q, want loopback default", got)
	}
}
