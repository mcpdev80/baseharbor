package main

import (
	"context"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// TestBindExplicitSelectionFailsClosedWithoutConnection proves Step 1 + Step 2:
// an explicit selection with no management connection fails closed with a typed
// error (not a nil/empty result) and no local effect. A nil issuer IS a missing
// connection. The missing-authority sentinel is mapped by the helper to a typed
// authentication-failed rejection on the production selection path.
func TestBindExplicitSelectionFailsClosedWithoutConnection(t *testing.T) {
	err := bindExplicitSelectionToConnection(
		context.Background(),
		"docker-dev",
		"core-prod",
		nil,
		serviceaccess.CertificateStatus{},
		"spiffe://baseharbor/platform/core/core-prod",
	)
	if err == nil {
		t.Fatal("explicit selection without a management connection must fail closed")
	}
	if machine.Classify(err).Code != machine.ErrorAuthenticationFailed {
		t.Fatalf("expected typed auth-failed rejection, got %v", err)
	}
}

// TestBindExplicitSelectionRequiresInstallation proves Step 1's separation: a
// Target with no installation ID is not a valid selection and fails closed with
// a typed conflict, before any verification work.
func TestBindExplicitSelectionRequiresInstallation(t *testing.T) {
	err := bindExplicitSelectionToConnection(
		context.Background(),
		"docker-dev",
		"",
		nil,
		serviceaccess.CertificateStatus{},
		"",
	)
	if err == nil {
		t.Fatal("target without an installation must fail closed")
	}
	if machine.Classify(err).Code != machine.ErrorConflict {
		t.Fatalf("expected typed conflict rejection, got %v", err)
	}
}
