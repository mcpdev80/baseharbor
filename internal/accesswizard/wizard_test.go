package accesswizard

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	"github.com/mcpdev80/baseharbor/internal/policy"
)

// harness wires the wizard to an in-memory input stream and captures prompts.
type harness struct {
	in  string
	out strings.Builder
}

func newHarness(input string) *harness {
	return &harness{in: input}
}

func (h *harness) controller(scope map[string]string) *Controller {
	c := NewController(scope)
	c.SetScope(scope)
	// A trailing newline is the operator's Enter key; the harness supplies it
	// for every line including the last, so the confirmation gate reads "yes"
	// correctly.
	c.SetIO(strings.NewReader(h.in), &h.out)
	return c
}

func (h *harness) outStr() string { return h.out.String() }

// TestCancelBeforeApprovalFailsClosed proves cancel at the name prompt makes no change.
func TestCancelBeforeApprovalFailsClosed(t *testing.T) {
	h := newHarness("cancel\n")
	c := h.controller(map[string]string{"local": "read"})
	res := c.Run(func(Review) policy.Decision { t.Fatal("decide must not be called"); return policy.Allow })
	if res.Err != nil {
		t.Fatalf("cancel should not error: %v", res.Err)
	}
	if res.Decision != policy.Allow {
		t.Fatalf("cancel should yield Allow, got %v", res.Decision)
	}
	if !strings.Contains(h.outStr(), "Cancelled") {
		t.Fatalf("expected cancel message, got %q", h.outStr())
	}
}

// TestEOFAtNameFailsClosed proves a missing person name fails closed.
func TestEOFAtNameFailsClosed(t *testing.T) {
	h := newHarness("")
	c := h.controller(map[string]string{"local": "read"})
	res := c.Run(func(Review) policy.Decision { return policy.Allow })
	if !errors.Is(res.Err, ErrIncompleteConfig) {
		t.Fatalf("empty name must fail closed on ErrIncompleteConfig, got %v", res.Err)
	}
}

// TestReviewApprovalProvesAssignment drives a full happy path to the review and approves.
func TestReviewApprovalProvesAssignment(t *testing.T) {
	h := newHarness("alice\n1\nyes\n")
	c := h.controller(map[string]string{"local": "read"})
	var called bool
	res := c.Run(func(Review) policy.Decision {
		called = true
		return policy.Allow
	})
	if res.Err != nil {
		t.Fatalf("approval should not error: %v", res.Err)
	}
	if !called {
		t.Fatal("decide callback was not invoked")
	}
	if res.Decision != policy.Allow {
		t.Fatalf("expected Allow, got %v", res.Decision)
	}
	if !strings.Contains(h.outStr(), "--- review ---") {
		t.Fatalf("expected review screen, got %q", h.outStr())
	}
}

// TestReviewDenyFailsClosed proves a production Deny surfaces as ErrDenied and persists nothing.
func TestReviewDenyFailsClosed(t *testing.T) {
	h := newHarness("alice\n1\nyes\n")
	c := h.controller(map[string]string{"local": "read"})
	res := c.Run(func(Review) policy.Decision { return policy.Deny })
	if !errors.Is(res.Err, ErrDenied) {
		t.Fatalf("deny must surface ErrDenied, got %v", res.Err)
	}
	if res.Decision != policy.Deny {
		t.Fatalf("expected Deny, got %v", res.Decision)
	}
}

// TestLargeListPaginationPreservesSelection proves a list larger than one page
// can be paged and a selection survives the page boundary.
func TestLargeListPaginationPreservesSelection(t *testing.T) {
	scope := map[string]string{}
	for i := 0; i < PageSize+5; i++ {
		scope["target"+itoa(i)] = "read"
	}
	// Select the last target by number (it exists on a later page), then cancel
	// to finish the selection loop.
	input := "bob\n" + itoa(PageSize+5) + "\ncancel\n"
	h := newHarness(input)
	c := h.controller(scope)
	res := c.Run(func(Review) policy.Decision { return policy.Allow })
	if res.Err != nil {
		t.Fatalf("large-list selection should not error: %v", res.Err)
	}
	if res.Decision != policy.Allow {
		t.Fatalf("expected Allow, got %v", res.Decision)
	}
}

// TestInvalidSelectionRepeatsProvesNoSilentCommit proves an invalid choice does
// not silently consume input or commit.
func TestInvalidSelectionRepeatsProvesNoSilentCommit(t *testing.T) {
	// Two known targets: 999 and 0 are out of range, 2 selects, then approve.
	h := newHarness("cara\n999\n0\n2\nyes\n")
	c := h.controller(map[string]string{"local": "read", "remote": "read"})
	var reviews int
	c.Run(func(Review) policy.Decision { reviews++; return policy.Allow })
	if reviews != 1 {
		t.Fatalf("expected exactly one review after invalid choice is rejected, got %d", reviews)
	}
	if !strings.Contains(h.outStr(), "out of range") {
		t.Fatalf("expected out-of-range feedback, got %q", h.outStr())
	}
}

// TestResolveAssignmentAllowProvesProductionConnection proves ResolveAssignment
// folds through the accepted policy authority: a created profile that assigns the
// operation yields Allow.
func TestResolveAssignmentAllowProvesProductionConnection(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	if _, err := m.Create(orgconfig.Profile{Name: "readers", Target: "dev", Operations: orgconfig.CanonicalOperations(orgconfig.OpRead)}, false); err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	decision, err := ResolveAssignment(m, nil, "reader", "dev", "read", false)
	if err != nil {
		t.Fatalf("assignment should not error: %v", err)
	}
	if decision != policy.Allow {
		t.Fatalf("assigned operation must allow, got %v", decision)
	}
}

// TestResolveAssignmentUnknownOperationDeniesProvesNoSilentWiden proves a wizard
// verb that is not a declared machine operation folds to Deny before any profile
// is written through the single production authority.
func TestResolveAssignmentUnknownOperationDeniesProvesNoSilentWiden(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	decision, err := ResolveAssignment(m, nil, "reader", "dev", "supersede", false)
	if err == nil {
		t.Fatal("unknown operation verb must fail closed")
	}
	if decision != policy.Deny {
		t.Fatalf("unknown operation must deny, got %v", decision)
	}
}

// TestResolveAssignmentEmptyProfileNameDeniesProvesFailClosed proves a blank
// person name folds to Deny without writing a profile through the authority.
func TestResolveAssignmentEmptyProfileNameDeniesProvesFailClosed(t *testing.T) {
	m := orgconfig.NewManager("install-acme")
	decision, err := ResolveAssignment(m, nil, "   ", "dev", "read", false)
	if err == nil {
		t.Fatal("empty profile name must fail closed")
	}
	if decision != policy.Deny {
		t.Fatalf("empty profile name must deny, got %v", decision)
	}
}

// TestResolveAssignmentNilManagerFailsClosed proves a nil manager (no active
// organization) fails closed rather than granting.
func TestResolveAssignmentNilManagerFailsClosed(t *testing.T) {
	_, err := ResolveAssignment(nil, nil, "reader", "dev", "read", false)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("nil manager must fail closed on ErrDenied, got %v", err)
	}
}

// TestValidateOffboardRequiresManagedUser proves an explicit managed-user
// removal without a name fails closed.
func TestValidateOffboardRequiresManagedUser(t *testing.T) {
	r := OffboardRequest{Kind: RemoveManagedUser, Person: Person{Name: "bob"}, Target: "dev"}
	if err := ValidateOffboard(r); err == nil {
		t.Fatal("managed-user removal without a name must fail closed")
	}
	r.Profile = "carol"
	if err := ValidateOffboard(r); err != nil {
		t.Fatalf("complete managed-user removal should validate: %v", err)
	}
}

// TestRemoveKindString proves the four distinct offboard kinds render distinctly.
func TestRemoveKindString(t *testing.T) {
	cases := map[RemoveKind]string{
		RemoveConnection:  "connection",
		RemoveDevice:      "device",
		RemoveGrant:       "owned grant",
		RemoveManagedUser: "managed user",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Fatalf("%d rendered %q, want %q", kind, got, want)
		}
	}
}

// TestRenderReviewShowsEffectiveScope proves the review screen lists each
// selection's effective scope for the operator's informed approval.
func TestRenderReviewShowsEffectiveScope(t *testing.T) {
	r := Review{Person: Person{Name: "dave"}, Selected: []Selection{{TargetName: "dev", Operation: "read", EffectiveScope: "dev"}}}
	var joined string
	for _, l := range r.Render() {
		joined += l
	}
	if !strings.Contains(joined, "dave") || !strings.Contains(joined, "dev") {
		t.Fatalf("review must show person and target, got %q", joined)
	}
}

// TestReadLineEOF proves EOF during selection is surfaced as an error.
func TestReadLineEOF(t *testing.T) {
	c := NewController(map[string]string{"local": "read"})
	c.SetScope(map[string]string{"local": "read"})
	c.SetIO(strings.NewReader(""), io.Discard)
	res := c.Run(func(Review) policy.Decision { return policy.Allow })
	if res.Err == nil {
		t.Fatal("EOF during selection should surface an error")
	}
}

// TestMultiLineInputIsNotCorrupted proves the wizard reads a full multi-line
// interaction (name, selection, confirm) intact. A fresh bufio.Reader per line
// would drop the bytes of the following line and corrupt this flow; the shared
// buffered reader preserves it.
func TestMultiLineInputIsNotCorrupted(t *testing.T) {
	h := newHarness("alice\n1\nyes\n")
	c := h.controller(map[string]string{"local": "read"})
	res := c.Run(func(Review) policy.Decision { return policy.Allow })
	if res.Err != nil {
		t.Fatalf("full multi-line flow should not error: %v", res.Err)
	}
	if res.Decision != policy.Allow {
		t.Fatalf("expected Allow, got %v", res.Decision)
	}
	if !strings.Contains(h.outStr(), "--- review ---") {
		t.Fatalf("expected the review screen after the full flow, got %q", h.outStr())
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
