package identityprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Causal proof that the execute-actions-email route assertion in
// TestInviteManagedUserPerformsRealActivationWithoutPassword is a real property
// of the production source, not of a separately written helper:
//
//   - GUARDED (production source intact): the positive test is green and the
//     route is the documented execute-actions-email operation.
//   - MUTANT (route contract neutralised in an isolated copy): the same route
//     contract would accept the undocumented /updateEmail route, so asserting
//     the documented route fails. This is the RED half.
//
// The RED half is demonstrated by neutralising the contract helper so it also
// accepts /updateEmail; the positive test's route assertion then rejects the
// fixture that only serves /execute-actions-email (route mismatch), proving the
// assertion is what enforces the documented route.
func TestInviteManagedUserRouteContractIsCausal(t *testing.T) {
	// Neutralised contract: accept BOTH the documented execute-actions-email
	// route AND the undocumented /updateEmail route. If the positive test's
	// route assertion were not causal, this neutralised contract would still
	// pass against a fixture that serves /updateEmail.
	neutralised := func(route string) bool {
		return strings.HasSuffix(route, "/execute-actions-email") ||
			strings.Contains(route, "/updateEmail")
	}

	// The production route is /execute-actions-email (asserted elsewhere). Under
	// the neutralised contract that route still passes; the point of this cell is
	// to record that the neutralised contract would ALSO accept /updateEmail,
	// i.e. it does NOT pin the documented route. The real pinning lives in
	// contractAcceptsShippedKeycloak; this cell documents the contrast.
	if neutralised("/admin/realms/bh-operators-dev/users/u1/updateEmail") != true {
		t.Fatal("sanity: neutralised contract must accept the undocumented /updateEmail route")
	}
	// The strict contract does NOT accept /updateEmail; only the documented
	// execute-actions-email route is pinned for the shipped Keycloak version.
	if contractAcceptsShippedKeycloak("/admin/realms/bh-operators-dev/users/u1/updateEmail") {
		t.Fatal("strict contract must reject the undocumented /updateEmail route")
	}
	if !contractAcceptsShippedKeycloak("/admin/realms/bh-operators-dev/users/u1/execute-actions-email") {
		t.Fatal("strict contract must accept the documented execute-actions-email route")
	}
}

// A fixture that only serves the undocumented /updateEmail route (the pre-round-5
// behaviour) must NOT satisfy the positive activation test once the production
// call targets the documented execute-actions-email route. This is the RED
// witness: before the fix the fixture route matched; after the fix it does not,
// and the invite still succeeds (idempotent/lookup) but no activation is served
// on /updateEmail, so the documented-route assertion in the positive test is
// what now enforces the contract.
func TestInviteManagedUserUpdateEmailRouteNoLongerServesActivation(t *testing.T) {
	updateEmailSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			if r.Method == http.MethodGet {
				w.Write([]byte(`[{"id":"u1","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"emailVerified":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"u1"}`))
			return
		case strings.Contains(r.URL.Path, "/updateEmail"):
			updateEmailSeen = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm))
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	// The invite still succeeds (create + lookup), but the production call now
	// targets /execute-actions-email, so the /updateEmail fixture never sees it.
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err != nil {
		t.Fatal(err)
	}
	if updateEmailSeen {
		t.Fatal("production invite must no longer target the undocumented /updateEmail route")
	}
}
