package controlplaneruntime

import (
	"errors"
	"strings"
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

func TestWorkstationOnlyEnrollmentUsesLocalCoreAuthority(t *testing.T) {
	cfg := Config{
		OIDCIssuer:                   "https://issuer.example",
		OIDCAudiences:                []string{"baseharbor"},
		DatabaseURL:                  "postgres://local/owned",
		WorkstationEnrollmentEnabled: true,
		ConnectorEnrollmentEnabled:   false,
		ConnectorAuthorityTarget:     "core-local",
	}
	// Reaching the TLS preflight means workstation-only mode accepted the
	// explicitly owned Core authority without requiring a Node Connector.
	if err := cfg.Validate(); !errors.Is(err, ErrMissingTLSFiles) {
		t.Fatalf("workstation-only authority was rejected before TLS: %v", err)
	}
	cfg.ConnectorAuthorityTarget = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "authority Target") {
		t.Fatalf("workstation enrollment without owned authority was accepted: %v", err)
	}
	cfg.WorkstationEnrollmentEnabled = false
	cfg.ConnectorAuthorityTarget = "core-local"
	if err := cfg.Validate(); err == nil {
		t.Fatal("an unused connector authority Target was accepted")
	}
}
