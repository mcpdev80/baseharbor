package applicationinput

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveDefaultsGeneratedExternalAndConditional(t *testing.T) {
	definitions := []Definition{
		{Name: "region", Class: ClassDefault, Default: "local", Required: true},
		{Name: "tls_mode", Class: ClassExternal, Required: true},
		{Name: "cert_dir", Class: ClassExternal, RequiredIf: &Condition{Input: "tls_mode", Equals: "existing"}},
		{Name: "runtime_password", Class: ClassGenerated, Secret: true, Required: true},
	}
	result, err := Resolve(definitions, map[string]string{"tls_mode": "existing"}, func(def Definition) (string, error) {
		if def.Name != "runtime_password" {
			return "", errors.New("unexpected generated input")
		}
		return "generated-secret", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Values["region"].Value; got != "local" {
		t.Fatalf("region = %q, want local", got)
	}
	if got := result.Values["runtime_password"].String(); got != "<redacted>" {
		t.Fatalf("secret String() = %q", got)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0].Name != "cert_dir" {
		t.Fatalf("unresolved = %#v, want cert_dir", result.Unresolved)
	}
}

func TestResolveConditionalInputNotRequiredWhenConditionDoesNotMatch(t *testing.T) {
	result, err := Resolve([]Definition{
		{Name: "tls_mode", Class: ClassExternal, Required: true},
		{Name: "cert_dir", Class: ClassExternal, RequiredIf: &Condition{Input: "tls_mode", Equals: "existing"}},
	}, map[string]string{"tls_mode": "local"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unresolved) != 0 {
		t.Fatalf("unresolved = %#v, want none", result.Unresolved)
	}
}

func TestPersistableValuesExcludesSecrets(t *testing.T) {
	result, err := Resolve([]Definition{
		{Name: "hostname", Class: ClassExternal, Required: true},
		{Name: "token", Class: ClassExternal, Secret: true, Required: true},
	}, map[string]string{"hostname": "app.example.test", "token": "super-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := PersistableValues(result), map[string]string{"hostname": "app.example.test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persistable = %#v, want %#v", got, want)
	}
	if got, want := SecretValues(result), map[string]string{"token": "super-secret"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("secrets = %#v, want %#v", got, want)
	}
}

func TestSuppliedValuesOverrideDefaultsAndGeneration(t *testing.T) {
	generated := false
	result, err := Resolve([]Definition{
		{Name: "environment", Class: ClassDefault, Default: "dev", Required: true},
		{Name: "password", Class: ClassGenerated, Secret: true, Required: true},
	}, map[string]string{"environment": "staging", "password": "injected"}, func(Definition) (string, error) {
		generated = true
		return "generated", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("generator called for explicitly supplied value")
	}
	if result.Values["environment"].Value != "staging" || result.Values["password"].Value != "injected" {
		t.Fatalf("resolved values = %#v", result.Values)
	}
}

func TestResolveRejectsDuplicateAndUnknownClass(t *testing.T) {
	if _, err := Resolve([]Definition{{Name: "x", Class: ClassExternal}, {Name: "x", Class: ClassExternal}}, nil, nil); err == nil {
		t.Fatal("expected duplicate input to fail")
	}
	if _, err := Resolve([]Definition{{Name: "x", Class: "magic"}}, nil, nil); err == nil {
		t.Fatal("expected unknown class to fail")
	}
}
