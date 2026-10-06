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
