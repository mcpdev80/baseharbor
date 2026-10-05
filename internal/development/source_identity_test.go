package development

import "testing"

func TestWorkspacePublicSourceIdentityRejectsCredentialsWithoutEchoingThem(t *testing.T) {
	for _, identity := range []string{"https://private-token@github.com/acme/api.git", "https://user:private-token@github.com/acme/api.git", "https://github.com/acme/api.git?token=private-token"} {
		if err := validatePublicSourceIdentity(identity); err == nil {
			t.Fatal("credential-bearing identity accepted")
		}
	}
	for _, identity := range []string{"https://github.com/acme/api.git", "git@github.com:acme/api.git", "ssh://git@github.com/acme/api.git"} {
		if err := validatePublicSourceIdentity(identity); err != nil {
			t.Fatalf("public Git identity rejected: %v", err)
		}
	}
}
