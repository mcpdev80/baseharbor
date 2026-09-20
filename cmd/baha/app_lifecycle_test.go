package main

import (
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestParseDestroyArgsRequiresExplicitConfirmationOnlyForMutation(t *testing.T) {
	name, confirmed, fullReset, err := parseDestroyArgs([]string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || confirmed || fullReset {
		t.Fatalf("unexpected preview parse result: name=%q confirmed=%t", name, confirmed)
	}

	name, confirmed, fullReset, err = parseDestroyArgs([]string{"--yes", "--full-reset", "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || !confirmed || !fullReset {
		t.Fatalf("unexpected confirmed parse result: name=%q confirmed=%t fullReset=%t", name, confirmed, fullReset)
	}
}

func TestParseDestroyArgsRejectsUnknownOption(t *testing.T) {
	if _, _, _, err := parseDestroyArgs([]string{"demo", "--force"}); err == nil {
		t.Fatal("expected unknown destroy option to fail")
	}
}

func TestOpenBaoDestroyScopeRequiredAllowsUninitializedCleanup(t *testing.T) {
	required, err := openBaoDestroyScopeRequired(openbao.State{Initialized: false})
	if err != nil {
		t.Fatal(err)
	}
	if required {
		t.Fatal("uninitialized OpenBao cannot contain a managed application scope")
	}
}

func TestOpenBaoDestroyScopeRequiredRejectsSealedState(t *testing.T) {
	required, err := openBaoDestroyScopeRequired(openbao.State{Initialized: true, Sealed: true})
	if required {
		t.Fatal("sealed OpenBao scope must not be destroyed without verification")
	}
	if !errors.Is(err, openbao.ErrSealed) {
		t.Fatalf("expected ErrSealed, got %v", err)
	}
}

func TestOpenBaoDestroyScopeRequiredUsesVerifiedInitializedState(t *testing.T) {
	required, err := openBaoDestroyScopeRequired(openbao.State{Initialized: true, Sealed: false})
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("initialized unsealed OpenBao requires verified application-scope destruction")
	}
}
