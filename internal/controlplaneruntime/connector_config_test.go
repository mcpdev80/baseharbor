package controlplaneruntime

import (
	"errors"
	"testing"
)

func TestEnrollmentConfigurationCannotExposeAnUnauthenticatedAuthority(t *testing.T) {
	for _, cfg := range []Config{
		{ConnectorEnrollmentEnabled: true},
		{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: "local", DatabaseURL: "postgres://db"},
		{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: "local", OIDCIssuer: "https://issuer.example", OIDCAudiences: []string{"core"}},
		{ConnectorAuthorityTarget: "local"},
		{ConnectorEnrollmentEnabled: true, ConnectorAuthorityTarget: "local", RuntimeAppName: "app", DatabaseURL: "postgres://db", OIDCIssuer: "https://issuer.example"},
	} {
		if err := cfg.Validate(); err == nil || errors.Is(err, ErrMissingTLSFiles) {
			t.Fatal("incomplete enrollment configuration did not fail at the authority boundary", err)
		}
	}
	if !errors.Is((Config{}).Validate(), ErrMissingTLSFiles) {
		t.Fatal("disabled enrollment changed the ordinary TLS requirement")
	}
}

func TestConnectorSessionsCannotStartWithUnboundAuthorityOrTLS(t *testing.T) {
	for _, cfg := range []Config{
		{ConnectorTLSCertFile: "unused"},
		{ConnectorListenAddr: "127.0.0.1:9443"},
		{ConnectorListenAddr: "127.0.0.1:9443", ConnectorEnrollmentEnabled: true},
		{ConnectorListenAddr: "127.0.0.1:9443", ConnectorEnrollmentEnabled: true, DatabaseURL: "postgres://db", OIDCIssuer: "https://issuer.example"},
		{ConnectorListenAddr: "127.0.0.1:9443", ConnectorEnrollmentEnabled: true, DatabaseURL: "postgres://db", OIDCIssuer: "https://issuer.example", RuntimeAppName: "app"},
	} {
		if err := validateConnectorTransport(cfg); err == nil {
			t.Fatal("incomplete remote authority accepted", cfg.ConnectorListenAddr)
		}
	}
	if err := validateConnectorTransport(Config{}); err != nil {
		t.Fatal("optional connector changed local startup", err)
	}
}
