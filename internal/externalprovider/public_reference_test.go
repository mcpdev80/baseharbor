package externalprovider

import (
	"path/filepath"
	"testing"
)

func TestPublicRegistrationPreservesDeclaredReferenceLocations(t *testing.T) {
	reg := registryIntegrityFixture()
	reg.CredentialRef = "vault://team/app/db"
	reg.Trust = Trust{Mode: TrustMTLS, CAReference: "file:///protected/ca.pem", ClientCertificate: "cert://team/db-client", ClientKey: "key://team/db-client", Directory: "/protected/link/../certificates"}
	store := Store{Path: filepath.Join(t.TempDir(), "providers.json")}
	if err := store.Register(reg); err != nil {
		t.Fatal(err)
	}
	inspected, err := store.Inspect(reg.ID)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 1 {
		t.Fatal("registered reference not observable")
	}
	for _, actual := range []Registration{reg.Public(), inspected, listed[0]} {
		if actual.CredentialRef != reg.CredentialRef || actual.Trust != reg.Trust {
			t.Fatal("public registration changed a declared credential or trust location")
		}
	}
}
