package coreinstallation

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// TestConnectionSelectionRequiresInstallationAndTarget proves Step 1's
// separation: a Target without an Installation (or a Connection without a
// Target) is not a valid selection and fails closed before any verification.
func TestConnectionSelectionRequiresInstallationAndTarget(t *testing.T) {
	tests := []struct {
		name      string
		selection ConnectionSelection
		wantErr   string
	}{
		{
			name:      "target only, no installation",
			selection: ConnectionSelection{TargetID: "docker-dev"},
			wantErr:   "requires an installation",
		},
		{
			name:      "installation and target equal",
			selection: ConnectionSelection{InstallationID: "core", TargetID: "core"},
			wantErr:   "distinct selections",
		},
		{
			name:      "installation only, no target",
			selection: ConnectionSelection{InstallationID: "core"},
			wantErr:   "requires a Target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "installation and target equal" {
				if err := tt.selection.AssertDistinct(); err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				return
			}
			if err := tt.selection.Validate(); err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
		})
	}
}

// TestConnectionSelectionDistinctProvesSeparation proves the three authorities
// are distinct: InstallationID and TargetID must never be equal. The
// management connection is the third, distinct authority, verified through the
// production boundary, not asserted here.
func TestConnectionSelectionDistinctProvesSeparation(t *testing.T) {
	s := ConnectionSelection{InstallationID: "core-prod", ConnectionID: "mgt-1", TargetID: "docker-dev"}
	if err := s.AssertDistinct(); err != nil {
		t.Fatalf("distinct selection rejected: %v", err)
	}
}

// TestConnectionSelectionResolveFailsClosedWithoutConnection proves Step 1's
// core claim: a Target without a management connection fails closed with a
// typed error and selects no local execution. A nil issuer IS a missing
// connection, so it must fail closed before any liveness work.
func TestConnectionSelectionResolveFailsClosedWithoutConnection(t *testing.T) {
	s := ConnectionSelection{InstallationID: "core-prod", ConnectionID: "mgt-1", TargetID: "docker-dev"}
	err := s.Resolve(context.Background(), nil, serviceaccess.CertificateStatus{}, "spiffe://baseharbor/platform/core/core-prod")
	if err == nil {
		t.Fatal("target without a management connection must fail closed")
	}
	if !strings.Contains(err.Error(), "missing management connection authority") {
		t.Fatalf("expected missing-authority sentinel, got %v", err)
	}
}

// TestConnectionSelectionResolveRejectsForeignConnection proves Step 3's
// installation-identity / connection-foreign mutation through the real
// production issuer boundary: ConnectionSelection.Resolve rejects a connection
// whose observed identity does not equal the authoritative connection identity
// as foreign, before any effect.
func TestConnectionSelectionResolveRejectsForeignConnection(t *testing.T) {
	iss, _ := realIssuer(t)
	obs := realObserved(t, iss)
	s := ConnectionSelection{InstallationID: "core-prod", ConnectionID: "mgt-1", TargetID: "docker-dev"}
	err := s.Resolve(context.Background(), iss, obs, "foreign-identity")
	if err == nil {
		t.Fatal("foreign connection identity must fail closed")
	}
}
