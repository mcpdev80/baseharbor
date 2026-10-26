package runtime

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestYAMLParserCompatibilityForComposeFallback(t *testing.T) {
	input := []byte(`defaults: &defaults
  image: postgres:17
  restart: unless-stopped
services:
  db:
    <<: *defaults
    environment:
      LEGACY_BOOL: yes
      YAML12_BOOL: true
    ports:
      - "5432:5432"
`)

	var document map[string]any
	if err := yaml.Unmarshal(input, &document); err != nil {
		t.Fatalf("unmarshal compose compatibility fixture: %v", err)
	}

	services, ok := document["services"].(map[string]any)
	if !ok {
		t.Fatalf("services type = %T, want map[string]any", document["services"])
	}
	db, ok := services["db"].(map[string]any)
	if !ok {
		t.Fatalf("services.db type = %T, want map[string]any", services["db"])
	}
	if got := db["image"]; got != "postgres:17" {
		t.Fatalf("merged image = %#v, want postgres:17", got)
	}
	if got := db["restart"]; got != "unless-stopped" {
		t.Fatalf("merged restart = %#v, want unless-stopped", got)
	}

	env, ok := db["environment"].(map[string]any)
	if !ok {
		t.Fatalf("environment type = %T, want map[string]any", db["environment"])
	}
	if got := env["LEGACY_BOOL"]; got != "yes" {
		t.Fatalf("legacy boolean token = %#v (%T), want string yes", got, got)
	}
	if got := env["YAML12_BOOL"]; got != true {
		t.Fatalf("YAML 1.2 boolean token = %#v (%T), want bool true", got, got)
	}

	ports, ok := db["ports"].([]any)
	if !ok || len(ports) != 1 || ports[0] != "5432:5432" {
		t.Fatalf("ports = %#v, want quoted string port", db["ports"])
	}
}

func TestYAMLParserCompatibilityForTypedLegacyBoolean(t *testing.T) {
	var value struct {
		Enabled bool `yaml:"enabled"`
	}
	if err := yaml.Unmarshal([]byte("enabled: yes\n"), &value); err != nil {
		t.Fatalf("unmarshal typed legacy boolean: %v", err)
	}
	if !value.Enabled {
		t.Fatal("typed YAML 1.1 boolean token yes must remain true for v3 compatibility")
	}
}
