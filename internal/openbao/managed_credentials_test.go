package openbao

import "testing"

func TestManagedCredentialReferenceValidation(t *testing.T) {
	for _, ref := range []string{
		"targets/local/human/developer",
		"applications/3d6b7d70-4cd3-4c78-8756-87f652cb39ac/sql/default",
		"providers/keycloak/admin",
	} {
		if _, err := validateManagedCredentialReference(ref); err != nil {
			t.Fatalf("%q: %v", ref, err)
		}
	}
	for _, ref := range []string{"", "/absolute", "../escape", "a//b", "a/../b", "a b"} {
		if _, err := validateManagedCredentialReference(ref); err == nil {
			t.Fatalf("invalid managed credential reference accepted: %q", ref)
		}
	}
}
