package auth

import (
	"errors"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		err  error
	}{
		{name: "valid", cfg: Config{Issuer: "https://auth.example.com/realms/baseharbor", Audiences: []string{"baseharbor"}}},
		{name: "missing issuer", cfg: Config{Audiences: []string{"baseharbor"}}, err: ErrInvalidIssuer},
		{name: "http issuer", cfg: Config{Issuer: "http://auth.example.com", Audiences: []string{"baseharbor"}}, err: ErrInvalidIssuer},
		{name: "issuer query", cfg: Config{Issuer: "https://auth.example.com?x=1", Audiences: []string{"baseharbor"}}, err: ErrInvalidIssuer},
		{name: "missing audience", cfg: Config{Issuer: "https://auth.example.com"}, err: ErrMissingAudience},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.err == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatalf("expected %v, got %v", tt.err, err)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	token, err := BearerToken("Bearer abc.def.ghi")
	if err != nil {
		t.Fatal(err)
	}
	if token != "abc.def.ghi" {
		t.Fatalf("unexpected token: %q", token)
	}

	token, err = BearerToken("bearer test-token")
	if err != nil || token != "test-token" {
		t.Fatalf("case-insensitive bearer parse failed: token=%q err=%v", token, err)
	}
}

func TestBearerTokenFailsClosed(t *testing.T) {
	if _, err := BearerToken(""); !errors.Is(err, ErrMissingAuthorization) {
		t.Fatalf("expected ErrMissingAuthorization, got %v", err)
	}
	for _, header := range []string{"Basic abc", "Bearer", "Bearer a b"} {
		if _, err := BearerToken(header); !errors.Is(err, ErrInvalidBearerFormat) {
			t.Fatalf("header %q: expected ErrInvalidBearerFormat, got %v", header, err)
		}
	}
}
