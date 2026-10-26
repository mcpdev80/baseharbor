package application

import "testing"

func TestDevelopmentPostgresUIEmailUsesGloballyValidAlias(t *testing.T) {
	if got, want := developmentPostgresUIEmail("developer"), "developer@baseharbor.dev"; got != want {
		t.Fatalf("development pgAdmin email = %q, want %q", got, want)
	}
	if got, want := developmentPostgresUIEmail("custom@example.com"), "custom@example.com"; got != want {
		t.Fatalf("explicit pgAdmin email = %q, want %q", got, want)
	}
}
