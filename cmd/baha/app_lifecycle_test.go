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

// TestParseDestroyArgsAcceptsUUIDSelector (0427-175): the registry-conflict
// repair advertises 'baha app destroy --uuid <id> --full-reset'. The option pair
// is consumed by extractApplicationUUID/extractApplicationEnvironment, so the
// positional parser must neither mistake it for a NAME nor reject it as an
// unknown option.
func TestParseDestroyArgsAcceptsUUIDSelector(t *testing.T) {
	// The command seam hands the positional scan the list that
	// extractApplicationEnvironment produced (the FLAG consumed, the bare VALUE
	// left behind), so both the pair form and the split form must parse.
	for _, raw := range [][]string{
		{"--uuid", "d-1", "--full-reset", "--yes"},
		{"--full-reset", "--yes"}, // same pair after the splitter consumed it
	} {
		name, confirmed, fullReset, err := parseDestroyArgs(mustSplit(t, raw...))
		if err != nil || name != "" || !confirmed || !fullReset {
			t.Fatalf("the advertised repair shape %v must parse as a flag pair: name=%q confirmed=%t fullReset=%t err=%v", raw, name, confirmed, fullReset, err)
		}
	}
	if name, _, _, err := parseDestroyArgs([]string{"--uuid=d-2"}); err != nil || name != "" {
		t.Fatalf("--uuid=<id> must parse as a flag pair: name=%q err=%v", name, err)
	}
	// A deployment id must never be taken for a NAME: doing so is what would
	// send the repair at a nonsense (or same-named sibling) deployment on a
	// poisoned Target.
	if name, _, _, err := parseDestroyArgs([]string{"--uuid", "d-3"}); err != nil || name != "" {
		t.Fatalf("the selector value must not land in the NAME slot: name=%q err=%v", name, err)
	}
	if name, _, _, err := parseDestroyArgs([]string{"demo"}); err != nil || name != "demo" {
		t.Fatalf("NAME must parse unchanged: name=%q err=%v", name, err)
	}
}

// mustSplit returns the positional list extractApplicationEnvironment produces
// for the given raw arguments, i.e. exactly what the destroy seam forwards.
func mustSplit(t *testing.T, args ...string) []string {
	t.Helper()
	filtered, _, err := extractApplicationEnvironment(args, "destroy")
	if err != nil {
		t.Fatalf("extractApplicationEnvironment(%v): %v", args, err)
	}
	return filtered
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
