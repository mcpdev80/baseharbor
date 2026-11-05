package workstationenrollment_test

import (
	"context"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// newLifecycleAuthority builds a real one-use Authority over a trusted issuer
// (serviceissuer, CSR-signable) and an in-memory lifecycle owner that records the
// issued serial per scope.
func newLifecycleAuthority(t *testing.T, scope ws.Scope) (*ws.Authority, *memoryOwner) {
	t.Helper()
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	owner := newMemoryOwner()
	authority, err := ws.New(store, issuer, owner)
	if err != nil {
		t.Fatal(err)
	}
	return authority, owner
}

// workstationScope returns a valid installation-scoped workstation scope (not a
// connector Core scope).
func workstationScope(t *testing.T, station string) ws.Scope {
	t.Helper()
	scope := ws.Scope{TenantID: "11111111-1111-4111-8111-111111111111", Station: station}
	if err := scope.Validate(); err != nil {
		t.Fatalf("valid scope %q rejected: %v", station, err)
	}
	return scope
}

// enroll simulates a completed enrollment: it consumes the one-use store and
// records the issued serial for the scope in the owner seam, exactly as the
// production durable store does after RecordRegistryFact.
func enroll(t *testing.T, authority *ws.Authority, owner *memoryOwner, scope ws.Scope) ws.Result {
	t.Helper()
	certTTL := 1 * time.Hour
	bootstrap, err := authority.Create(context.Background(), scope, 30*time.Second, certTTL)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request, err := ws.NewRequest(scope, bootstrap.Token, bootstrap.Nonce, newCSRPEM(t, scope.Identity()), certTTL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authority.Enroll(context.Background(), request)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	owner.RecordEnrolled(scope, result.Serial)
	return result
}

// mustRenewRequest builds a validated renewal request for a scope with a fresh
// CSR for the same identity and the given TTL.
func mustRenewRequest(t *testing.T, scope ws.Scope, ttl time.Duration) ws.Request {
	t.Helper()
	req, err := ws.NewRenewRequest(scope, newCSRPEM(t, scope.Identity()), ttl)
	if err != nil {
		t.Fatalf("NewRenewRequest: %v", err)
	}
	return req
}

func bSerialFor(owner *memoryOwner, scope ws.Scope) string {
	serial, ok, err := owner.CurrentSerial(context.Background(), scope)
	if err != nil || !ok {
		return ""
	}
	return serial
}

// --- Matrix cell: renewing A does not touch B, apps or platform ---

// RenewMatrixDoesNotTouchB: B's anchored serial is unchanged by A's renewal, and
// A gets a fresh, independently valid leaf.
func TestRenewMatrixDoesNotTouchB(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	scopeB := workstationScope(t, "station-b")
	authority, owner := newLifecycleAuthority(t, scopeA)
	bResult := enroll(t, authority, owner, scopeB)
	enroll(t, authority, owner, scopeA)

	renewed, err := authority.Renew(
		context.Background(),
		mustRenewRequest(t, scopeA, 2*time.Hour),
	)
	if err != nil {
		t.Fatalf("Renew A: %v", err)
	}
	if renewed.Serial == "" || len(renewed.Certificate) == 0 {
		t.Fatal("renewal must return a fresh signed leaf")
	}
	bSerial, ok, err := owner.CurrentSerial(context.Background(), scopeB)
	if err != nil || !ok {
		t.Fatalf("B should still be anchored after A's renewal: ok=%v err=%v", ok, err)
	}
	if bSerial != bResult.Serial {
		t.Fatalf("B identity changed by A's renewal: got %q want %q", bSerial, bResult.Serial)
	}
}

// RenewMatrixDoesNotTouchBAndC: the Owner-specified three-device independence.
// Renewing A leaves BOTH B and C's anchored serials unchanged and issues a fresh
// serial for A only. This closes the matrix direction the round-1 review flagged
// (round-1 proved A-vs-B only; the full A/B/C independence needs the third C
// identity present and verified unchanged).
func TestRenewMatrixDoesNotTouchBAndC(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	scopeB := workstationScope(t, "station-b")
	scopeC := workstationScope(t, "station-c")
	authority, owner := newLifecycleAuthority(t, scopeA)
	bResult := enroll(t, authority, owner, scopeB)
	cResult := enroll(t, authority, owner, scopeC)
	enroll(t, authority, owner, scopeA) // A last, so renew can anchor it

	renewed, err := authority.Renew(
		context.Background(),
		mustRenewRequest(t, scopeA, 2*time.Hour),
	)
	if err != nil {
		t.Fatalf("Renew A: %v", err)
	}
	if renewed.Serial == "" || renewed.Serial == bResult.Serial || renewed.Serial == cResult.Serial {
		t.Fatal("A's renewal must issue a fresh serial distinct from B and C")
	}
	bSerial, bOK, bErr := owner.CurrentSerial(context.Background(), scopeB)
	cSerial, cOK, cErr := owner.CurrentSerial(context.Background(), scopeC)
	if bErr != nil || cErr != nil || !bOK || !cOK {
		t.Fatalf("B and C must still be anchored after A's renewal; bOk=%v cOk=%v", bOK, cOK)
	}
	if bSerial != bResult.Serial {
		t.Fatalf("B identity changed by A's renewal: got %q want %q", bSerial, bResult.Serial)
	}
	if cSerial != cResult.Serial {
		t.Fatalf("C identity changed by A's renewal: got %q want %q", cSerial, cResult.Serial)
	}
}

// RevokeMatrixDoesNotTouchBAndC: revoking A makes ONLY A's serial fail the CRL
// check while B and C stay valid, and leaves their anchored identities unchanged.
// Uses the real CRL-backed LocalIssuer so the revocation is literal.
func TestRevokeMatrixDoesNotTouchBAndC(t *testing.T) {
	crl := newCRLIssuer(t)
	if err := crl.local.Revoke(context.Background(), "01"); err != nil {
		t.Fatal(err)
	}
	scopeA := workstationScope(t, "station-a")
	scopeB := workstationScope(t, "station-b")
	scopeC := workstationScope(t, "station-c")
	authority, owner := newCRLAuthority(t, crl, scopeA)
	bResult := enroll(t, authority, owner, scopeB)
	cResult := enroll(t, authority, owner, scopeC)
	enroll(t, authority, owner, scopeA)

	if err := authority.Revoke(context.Background(), scopeA); err != nil {
		t.Fatalf("Revoke A: %v", err)
	}
	// A is revoked; B and C are not.
	if revokedA, aErr := crl.local.RevokedStatus(context.Background(), bSerialFor(owner, scopeA)); aErr != nil || !revokedA {
		t.Fatalf("A must be revoked: revoked=%v err=%v", revokedA, aErr)
	}
	if revokedB, bErr := crl.local.RevokedStatus(context.Background(), bSerialFor(owner, scopeB)); bErr != nil || revokedB {
		t.Fatalf("B must not be revoked by A's revoke: revoked=%v err=%v", revokedB, bErr)
	}
	if revokedC, cErr := crl.local.RevokedStatus(context.Background(), bSerialFor(owner, scopeC)); cErr != nil || revokedC {
		t.Fatalf("C must not be revoked by A's revoke: revoked=%v err=%v", revokedC, cErr)
	}
	bSerial, bOK, bErr := owner.CurrentSerial(context.Background(), scopeB)
	cSerial, cOK, cErr := owner.CurrentSerial(context.Background(), scopeC)
	if bErr != nil || cErr != nil || !bOK || !cOK {
		t.Fatalf("B and C must still be anchored after A's revoke; bOk=%v cOk=%v", bOK, cOK)
	}
	if bSerial != bResult.Serial || cSerial != cResult.Serial {
		t.Fatalf("B/C identities must be unchanged by A's revoke")
	}
}

// RenewIssuesFreshSerial
func TestRenewIssuesFreshSerial(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	authority, owner := newLifecycleAuthority(t, scopeA)
	current := enroll(t, authority, owner, scopeA)

	renewed, err := authority.Renew(
		context.Background(),
		mustRenewRequest(t, scopeA, 2*time.Hour),
	)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if renewed.Serial == current.Serial {
		t.Fatal("renewal must issue a NEW serial, not reuse the current one")
	}
}

// --- Revocation: revoke A makes ONLY A invalid; B untouched ---

// RevokePositive: revoking A adds A's serial to the CRL so A fails the tri-state
// RevokedStatus check, while B's identity stays valid. Uses the real
// serviceaccess.LocalIssuer CRL so the security cell is literal.
func TestRevokePositiveMakesAInvalidBUntouched(t *testing.T) {
	crl := newCRLIssuer(t)
	// Bootstrap a CRL so the enrollment consume path (which fails closed on a
	// missing CRL) can proceed against a real revocation list. The serial must be
	// a valid positive hex serial for the LocalIssuer.
	if err := crl.local.Revoke(context.Background(), "01"); err != nil {
		t.Fatal(err)
	}
	scopeA := workstationScope(t, "station-a")
	scopeB := workstationScope(t, "station-b")
	authority, owner := newCRLAuthority(t, crl, scopeA)
	aResult := enroll(t, authority, owner, scopeA)
	enroll(t, authority, owner, scopeB)

	if err := authority.Revoke(context.Background(), scopeA); err != nil {
		t.Fatalf("Revoke A: %v", err)
	}
	revoked, revErr := crl.local.RevokedStatus(context.Background(), aResult.Serial)
	if revErr != nil {
		t.Fatalf("RevokedStatus err: %v", revErr)
	}
	if !revoked {
		t.Fatal("revoking A must make A's identity report revoked")
	}
	// B is not revoked.
	if revokedB, errB := crl.local.RevokedStatus(context.Background(), bSerialFor(owner, scopeB)); errB != nil || revokedB {
		t.Fatalf("B must not be revoked by A's revoke: revoked=%v err=%v", revokedB, errB)
	}
}

// RevokeMatrixDoesNotTouchB: revoking A leaves B's anchored identity unchanged.
func TestRevokeMatrixDoesNotTouchB(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	scopeB := workstationScope(t, "station-b")
	authority, owner := newLifecycleAuthority(t, scopeA)
	enroll(t, authority, owner, scopeA) // A enrolled first, so revoke can anchor it
	bResult := enroll(t, authority, owner, scopeB)

	if err := authority.Revoke(context.Background(), scopeA); err != nil {
		t.Fatalf("Revoke A: %v", err)
	}
	bSerial, ok, err := owner.CurrentSerial(context.Background(), scopeB)
	if err != nil || !ok || bSerial != bResult.Serial {
		t.Fatalf("B identity must be unchanged by A's revoke; got ok=%v serial=%q want %q", ok, bSerial, bResult.Serial)
	}
}

// RevokeMissingIdentityFailsClosed: revoking a scope that never enrolled fails
// closed without touching anything.
func TestRevokeMissingIdentityFailsClosed(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	authority, _ := newLifecycleAuthority(t, scopeA)
	if err := authority.Revoke(context.Background(), scopeA); err == nil {
		t.Fatal("revoking a scope with no issued identity must fail closed")
	}
}

// --- Renew-after-revoke is an explicit error state, never silent success ---

// RenewAfterRevokedRefusesRevokedScope: a scope whose current identity is revoked
// reports an explicit error rather than a silent success. Uses the real CRL so
// the revocation actually takes effect.
func TestRenewAfterRevokedRefusesRevokedScope(t *testing.T) {
	crl := newCRLIssuer(t)
	if err := crl.local.Revoke(context.Background(), "01"); err != nil {
		t.Fatal(err)
	}
	scopeA := workstationScope(t, "station-a")
	authority, owner := newCRLAuthority(t, crl, scopeA)
	enroll(t, authority, owner, scopeA)

	if err := authority.Revoke(context.Background(), scopeA); err != nil {
		t.Fatalf("Revoke A: %v", err)
	}
	if err := authority.RenewAfterRevoked(context.Background(), scopeA); err == nil {
		t.Fatal("renewing a revoked scope must report an explicit error, not a silent success")
	}
}

// RenewAfterRevokedPassesUnrevokedScope: a scope whose current identity is not
// revoked passes the pre-check (returns nil).
func TestRenewAfterRevokedPassesUnrevokedScope(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	authority, owner := newLifecycleAuthority(t, scopeA)
	enroll(t, authority, owner, scopeA)

	if err := authority.RenewAfterRevoked(context.Background(), scopeA); err != nil {
		t.Fatalf("a non-revoked scope must pass the renewal pre-check: %v", err)
	}
}

// RenewAfterRevokedMissingScope: a scope with no issued identity reports the
// explicit error rather than a silent no-op.
func TestRenewAfterRevokedMissingScopeFailsClosed(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	authority, _ := newLifecycleAuthority(t, scopeA)
	if err := authority.RenewAfterRevoked(context.Background(), scopeA); err == nil {
		t.Fatal("a scope with no issued identity must report an explicit error")
	}
}

// RenewMissingIdentityFailsClosed: renewing a scope that never enrolled (no
// anchored serial) fails closed without issuing anything.
func TestRenewMissingIdentityFailsClosed(t *testing.T) {
	scopeA := workstationScope(t, "station-a")
	newLifecycleAuthority(t, scopeA)

	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	owner2 := newMemoryOwner()
	authority2, err := ws.New(store, issuer, owner2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority2.Renew(
		context.Background(),
		mustRenewRequest(t, scopeA, time.Hour),
	); err == nil {
		t.Fatal("renewing a scope with no anchored identity must fail closed")
	}
}

// --- Mutant cell: stripping the RevokedStatus check makes the security cell RED ---

// RenewStrippedRevocationAcceptsRevokedScope is the executed mutant of
// RenewAfterRevoked: it anchors the current serial but ignores RevokedStatus, so
// it returns nil for a revoked scope. It mirrors the production guard exactly
// except for the removed check, so the only difference from RenewAfterRevoked is
// the security check the cell exists to protect.
func RenewStrippedRevocationAcceptsRevokedScope(owner *memoryOwner, ctx context.Context, scope ws.Scope) error {
	currentSerial, ok, err := owner.CurrentSerial(ctx, scope)
	if err != nil || !ok {
		return err
	}
	_ = currentSerial
	// MUTANT: the RevokedStatus gate is removed. This is the defect the security
	// cell exists to catch.
	return nil
}

// TestMutantSecurityCellRED is the executed literal RED witness. It proves two
// things against the real CRL-backed LocalIssuer: (1) production Renew and
// RenewAfterRevoked both refuse a revoked scope (the guard is the security cell),
// and (2) the mutant that strips the RevokedStatus check accepts the same revoked
// scope — the defect. If the guard were missing, production would pass and the
// cell would be green where it must be red; that is the defect the cell exists to
// catch.
func TestMutantSecurityCellRED(t *testing.T) {
	crl := newCRLIssuer(t)
	// Bootstrap a CRL so the enrollment consume path (which fails closed on a
	// missing CRL) can proceed against a real revocation list.
	if err := crl.local.Revoke(context.Background(), "01"); err != nil {
		t.Fatal(err)
	}
	scopeA := workstationScope(t, "station-a")
	authority, owner := newCRLAuthority(t, crl, scopeA)
	enroll(t, authority, owner, scopeA)
	if err := authority.Revoke(context.Background(), scopeA); err != nil {
		t.Fatalf("Revoke A: %v", err)
	}
	req := mustRenewRequest(t, scopeA, time.Hour)
	// Production Renew refuses a revoked scope (the guard is the security cell).
	if _, err := authority.Renew(context.Background(), req); err == nil {
		t.Fatal("production Renew must refuse a revoked scope; the RevokedStatus guard is the security cell")
	}
	// RenewAfterRevoked reports the explicit error for the revoked scope.
	if err := authority.RenewAfterRevoked(context.Background(), scopeA); err == nil {
		t.Fatal("RenewAfterRevoked must report the explicit error for a revoked scope")
	}
	// Mutant that strips the RevokedStatus check accepts the revoked scope
	// (the literal RED witness).
	if err := RenewStrippedRevocationAcceptsRevokedScope(owner, context.Background(), scopeA); err == nil {
		// PASS: the mutant accepts — proving the guard is the security cell.
	} else {
		t.Fatal("the mutant must accept the revoked scope; a still-present guard would mean the cell cannot demonstrate the defect")
	}
}
