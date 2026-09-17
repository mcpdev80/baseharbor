package main

import (
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationinput"
)

func TestExtractDeclaredInputArgs(t *testing.T) {
	forwarded, injected, err := extractDeclaredInputArgs([]string{
		"--yes",
		"--input", "hostname=app.example.test",
		"--input=tls_mode=existing",
		"--cert-dir", "/operator/certs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--yes", "--cert-dir", "/operator/certs"}; !reflect.DeepEqual(forwarded, want) {
		t.Fatalf("forwarded = %#v, want %#v", forwarded, want)
	}
	if injected[inputHostname] != "app.example.test" || injected[inputTLSMode] != "existing" {
		t.Fatalf("injected = %#v", injected)
	}
}

func TestExtractDeclaredInputArgsRejectsDuplicateAndMalformedValues(t *testing.T) {
	if _, _, err := extractDeclaredInputArgs([]string{"--input", "hostname=a", "--input", "hostname=b"}); err == nil {
		t.Fatal("expected duplicate input to fail")
	}
	if _, _, err := extractDeclaredInputArgs([]string{"--input", "hostname"}); err == nil {
		t.Fatal("expected malformed input to fail")
	}
}

func TestApplyInjectedRepositoryInputsRejectsConflictingLegacyFlag(t *testing.T) {
	opts := repositoryInitOptions{Hostname: "flag.example.test"}
	if err := applyInjectedRepositoryInputs(&opts, map[string]string{inputHostname: "input.example.test"}); err == nil {
		t.Fatal("expected conflicting injection to fail")
	}
}

func TestRepositoryDeploymentInputDefinitionsRequireExistingCertificateDirectoryConditionally(t *testing.T) {
	definitions := repositoryDeploymentInputDefinitions(true)
	result, err := applicationinput.Resolve(definitions, map[string]string{
		inputHostname: "app.example.test",
		inputTLSMode:  "existing",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0].Name != inputCertDir {
		t.Fatalf("unresolved = %#v, want cert_dir", result.Unresolved)
	}

	result, err = applicationinput.Resolve(definitions, map[string]string{
		inputHostname: "app.example.test",
		inputTLSMode:  "acme",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unresolved) != 0 {
		t.Fatalf("unresolved = %#v, want none", result.Unresolved)
	}
}
