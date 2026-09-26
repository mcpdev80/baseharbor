package runtime

import "testing"

func TestOperatorVisibleProjectNames(t *testing.T) {
	if got, want := SharedProjectName(""), "bh-local-shared"; got != want {
		t.Fatalf("SharedProjectName() = %q, want %q", got, want)
	}
	if got, want := SharedProjectName("prod.eu"), "bh-prod-eu-shared"; got != want {
		t.Fatalf("SharedProjectName(prod.eu) = %q, want %q", got, want)
	}
	if got, want := ApplicationProjectName("", "demo"), "bh-local-demo"; got != want {
		t.Fatalf("ApplicationProjectName() = %q, want %q", got, want)
	}
	if got, want := ApplicationProjectName("prod", "loki"), "bh-prod-loki-app"; got != want {
		t.Fatalf("reserved application project = %q, want %q", got, want)
	}
}
