package openbao

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationPolicyIsExactToApplicationAndEnvironment(t *testing.T) {
	alpha := ApplicationIdentity{Name: "alpha", Environment: "dev"}
	beta := ApplicationIdentity{Name: "beta", Environment: "dev"}
	policy := applicationPolicy(alpha)

	if !strings.Contains(policy, `path "baseharbor/data/apps/alpha/dev"`) {
		t.Fatal("policy does not contain the application's exact data path")
	}
	if strings.Contains(policy, "apps/alpha/dev/*") {
		t.Fatal("policy unexpectedly grants a wildcard below the application secret document")
	}
	if strings.Contains(policy, applicationSecretPath(beta)) {
		t.Fatal("policy contains another application's secret path")
	}
}

func TestApplicationIdentityNamesAreDeterministicAndSeparated(t *testing.T) {
	dev := ApplicationIdentity{Name: "mailflow", Environment: "dev"}
	prod := ApplicationIdentity{Name: "mailflow", Environment: "prod"}
	if applicationPolicyName(dev) == applicationPolicyName(prod) {
		t.Fatal("environments must not share an OpenBao policy name")
	}
	if applicationRoleName(dev) != "baseharbor-app-mailflow-dev" {
		t.Fatalf("unexpected role name %q", applicationRoleName(dev))
	}
	if applicationSecretPath(prod) != "apps/mailflow/prod" {
		t.Fatalf("unexpected secret path %q", applicationSecretPath(prod))
	}
}

func TestApplicationCredentialsAreOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "openbao.env")
	credentials := ApplicationCredentials{RoleID: "role", SecretID: "secret"}
	if err := writeApplicationCredentials(path, credentials); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 credentials, got %o", info.Mode().Perm())
	}
	loaded, err := loadApplicationCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != credentials {
		t.Fatalf("unexpected credentials %#v", loaded)
	}
}

func TestLoadApplicationCredentialsRejectsBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openbao.env")
	if err := os.WriteFile(path, []byte("OPENBAO_ROLE_ID=role\nOPENBAO_SECRET_ID=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadApplicationCredentials(path); err == nil {
		t.Fatal("expected broad permissions to be rejected")
	}
}

func TestApplicationIdentityRejectsUnsafeNames(t *testing.T) {
	for _, identity := range []ApplicationIdentity{
		{Name: "../other", Environment: "dev"},
		{Name: "alpha", Environment: "DEV"},
		{Name: "-alpha", Environment: "dev"},
	} {
		if err := validateApplicationIdentity(identity); err == nil {
			t.Fatalf("expected identity %#v to be rejected", identity)
		}
	}
}
