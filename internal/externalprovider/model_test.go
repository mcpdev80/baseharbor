package externalprovider

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestRegistrationRejectsCredentialsInEndpoint(t *testing.T) {
	r := Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://user:secret@db.example:5432/app",
		Provider: capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:    Trust{Mode: TrustSystem},
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected embedded credentials to fail")
	}
}

func TestRegistrationAcceptsReferenceOnlyTrust(t *testing.T) {
	r := Registration{
		ID: "company-db", ProviderID: "company/postgresql", Endpoint: "postgres://db.example:5432/app",
		CredentialRef: "vault://team/app/db",
		Provider:      capability.Provider{Kind: capability.ProviderKind("company-postgresql"), Capabilities: []capability.Kind{capability.SQL}},
		Trust:         Trust{Mode: TrustMTLS, CAReference: "corp-root", ClientCertificate: "cert://db-client", ClientKey: "key://db-client"},
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
