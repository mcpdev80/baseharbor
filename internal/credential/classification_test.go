package credential

import "testing"

func TestCredentialClassTaxonomy(t *testing.T) {
	tests := []struct {
		value     string
		want      Class
		shareable bool
		machine   bool
	}{
		{"human-management", ClassHumanManagement, true, false},
		{"application-service", ClassApplicationService, true, false},
		{"internal-machine", ClassInternalMachine, false, true},
	}

	for _, tt := range tests {
		got, err := ParseClass(tt.value)
		if err != nil {
			t.Fatalf("ParseClass(%q): %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("ParseClass(%q) = %q, want %q", tt.value, got, tt.want)
		}
		if got.Shareable() != tt.shareable {
			t.Fatalf("%q Shareable() = %v, want %v", got, got.Shareable(), tt.shareable)
		}
		if got.IsMachineIdentity() != tt.machine {
			t.Fatalf("%q IsMachineIdentity() = %v, want %v", got, got.IsMachineIdentity(), tt.machine)
		}
	}
}

func TestCredentialClassRejectsUnknownValues(t *testing.T) {
	for _, value := range []string{"", "human", "service", "machine", "provider-admin"} {
		if _, err := ParseClass(value); err == nil {
			t.Fatalf("ParseClass(%q) succeeded, want error", value)
		}
	}
}

func TestInternalMachineCredentialCannotBeShared(t *testing.T) {
	if ClassInternalMachine.Shareable() {
		t.Fatal("internal machine credentials must never be shareable")
	}
}
