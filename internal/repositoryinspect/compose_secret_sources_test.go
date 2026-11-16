package repositoryinspect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadComposeSecretSourcePolicies(t *testing.T) {
	root := t.TempDir()
	compose := `services:
  app:
    image: example/app
x-baseharbor:
  secrets:
    - name: DEMO_MANAGEMENT_USERNAME
      source: target-development-username
    - name: DEMO_MANAGEMENT_PASSWORD
      source: target-development-password
`
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadComposeSecretSourcePolicies(root, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"DEMO_MANAGEMENT_USERNAME": SecretSourceTargetDevelopmentUsername,
		"DEMO_MANAGEMENT_PASSWORD": SecretSourceTargetDevelopmentPassword,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("policies = %#v, want %#v", got, want)
	}
}

func TestReadComposeSecretSourcePoliciesRejectsInvalidAndDuplicateDeclarations(t *testing.T) {
	for _, compose := range []string{
		"x-baseharbor:\n  secrets:\n    - name: lower_case\n      source: target-development-username\n",
		"x-baseharbor:\n  secrets:\n    - name: PASSWORD\n      source: arbitrary-secret\n",
		"x-baseharbor:\n  secrets:\n    - name: PASSWORD\n      source: target-development-password\n    - name: PASSWORD\n      source: target-development-password\n",
	} {
		path := filepath.Join(t.TempDir(), "compose.yaml")
		if err := os.WriteFile(path, []byte(compose), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadComposeSecretSourcePolicies(filepath.Dir(path), filepath.Base(path)); err == nil {
			t.Fatalf("invalid Compose declarations accepted:\n%s", compose)
		}
	}
}
