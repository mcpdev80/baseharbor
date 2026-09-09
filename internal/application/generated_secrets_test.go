package application

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGeneratedSecretManifestRoundTrip(t *testing.T) {
	input := `version: 1
app:
  name: demo
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: true
secrets:
  required:
    - name: SECRET_KEY
      generate:
        type: random
        length: 64
    - name: ENCRYPTION_KEY
      generate:
        type: hex
        bytes: 32
    - name: OPENAI_API_KEY
`
	m, err := ParseYAML(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(GeneratedSecretRequirements(m)) != 2 {
		t.Fatalf("generated requirements = %d", len(GeneratedSecretRequirements(m)))
	}
	if got := m.YAML(); got != input {
		t.Fatalf("round trip mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, input)
	}
}

func TestGeneratedSecretValidationRejectsUnsafeShapes(t *testing.T) {
	cases := []SecretGeneration{
		{Type: "random", Length: 8},
		{Type: "random", Length: 64, Bytes: 32},
		{Type: "hex", Bytes: 8},
		{Type: "hex", Bytes: 32, Length: 64},
		{Type: "uuid", Length: 64},
	}
	for _, generation := range cases {
		m := WithGeneratedSecret(New("demo", "dev", true, false, true), "SECRET_KEY", generation.Type, 64)
		m.Secrets.Required[0].Generate = &generation
		if err := m.Validate(); err == nil {
			t.Fatalf("expected validation failure for %#v", generation)
		}
	}
}

func TestGenerateSecretValueRandom(t *testing.T) {
	first, err := GenerateSecretValue(SecretGeneration{Type: "random", Length: 64})
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateSecretValue(SecretGeneration{Type: "random", Length: 64})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 {
		t.Fatalf("length = %d", len(first))
	}
	if string(first) == string(second) {
		t.Fatal("two generated values unexpectedly match")
	}
	for _, r := range string(first) {
		if !strings.ContainsRune(generatedSecretAlphabet, r) {
			t.Fatalf("unexpected generated character %q", r)
		}
	}
}

func TestGenerateSecretValueHex(t *testing.T) {
	value, err := GenerateSecretValue(SecretGeneration{Type: "hex", Bytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 64 {
		t.Fatalf("encoded length = %d", len(value))
	}
	decoded, err := hex.DecodeString(string(value))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded length = %d", len(decoded))
	}
}
