package application

import "testing"

func TestManagementAuthenticationClasses(t *testing.T) {
	for _, class := range []ManagementAuthenticationClass{
		ManagementAuthNativeOIDC,
		ManagementAuthStandardsAdapter,
		ManagementAuthNativeCredential,
		ManagementAuthUnsupported,
	} {
		if err := ValidateManagementAuthenticationClass(class); err != nil {
			t.Fatalf("class %q: %v", class, err)
		}
	}
	if err := ValidateManagementAuthenticationClass("basic"); err == nil {
		t.Fatal("unknown management authentication class accepted")
	}
}

func TestNativeCredentialRoleMappingDoesNotPretendFineGrainedAuthorization(t *testing.T) {
	mappings := NativeCredentialRoleMappings("administrator", "user")
	if err := ValidateManagementRoleMappings(mappings); err != nil {
		t.Fatal(err)
	}
	if mappings[1].Level != ManagementRoleMappingLimited || mappings[3].Level != ManagementRoleMappingUnsupported {
		t.Fatalf("native role mapping overstates provider authorization: %#v", mappings)
	}
}
