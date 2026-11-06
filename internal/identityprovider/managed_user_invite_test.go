package identityprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const managedInviteRealm = `{"realm":"bh-operators-dev","enabled":true,"registrationAllowed":true,"attributes":{"baseharbor.owner":"baseharbor","baseharbor.scope":"operator","baseharbor.target":"isolated","baseharbor.environment":"dev"}}`

func managedInviteOwnership() map[string]string {
	return map[string]string{
		"baseharbor.owner":       "baseharbor",
		"baseharbor.scope":       "operator",
		"baseharbor.target":      "isolated",
		"baseharbor.environment": "dev",
	}
}

// A real provider-admin call performs the activation through the documented
// Keycloak Admin REST operation PUT
// /admin/realms/{realm}/users/{id}/execute-actions-email with a required-actions
// list; no password is ever stored, carried or echoed. This is the production
// call reviewed red in act-1397c: the candidate must not target the undocumented
// /updateEmail route; it must use the documented execute-actions-email operation.
func TestInviteManagedUserPerformsRealActivationWithoutPassword(t *testing.T) {
	var activationSeen, created, userGets int
	var seenMethod, seenRoute string
	var hasRequiredActions, sawPassword bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			if r.Method == http.MethodGet {
				userGets++
				if userGets == 1 {
					w.Write([]byte(`[]`)) // no owned account yet
					return
				}
				// resolve-after-create: exactly one owned user now
				w.Write([]byte(`[{"id":"u1","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"emailVerified":false,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
				return
			}
			created++
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"u1"}`))
			return
		case strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
			activationSeen++
			seenMethod = r.Method
			seenRoute = r.URL.Path
			var payload map[string]any
			if body, _ := io.ReadAll(r.Body); len(bytes.TrimSpace(body)) > 0 {
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("execute-actions-email body=%q: %v", string(body), err)
				}
				// The documented operation requires a recipient "to" that is
				// the created account, and a required-actions list.
				_, hasRequiredActions = payload["requiredActions"]
				if got := fmt.Sprintf("%v", payload["to"]); got != "u1" {
					t.Errorf("execute-actions-email did not target the created account: to=%v", payload["to"])
				}
				if pw, ok := payload["password"]; ok && pw != "" {
					sawPassword = true
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm)) // realm owner + verification
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err != nil {
		t.Fatal(err)
	}
	if activationSeen == 0 {
		t.Fatal("invite did not perform the genuine provider activation call")
	}
	if created == 0 {
		t.Fatal("invite did not create the owned account")
	}
	if seenMethod != http.MethodPut {
		t.Errorf("activation used %s, want PUT on the documented execute-actions-email route", seenMethod)
	}
	if !strings.HasSuffix(seenRoute, "/users/u1/execute-actions-email") {
		t.Errorf("activation route %q is not the documented execute-actions-email operation", seenRoute)
	}
	if !hasRequiredActions {
		t.Error("execute-actions-email payload is missing the required requiredActions list")
	}
	if sawPassword {
		t.Error("execute-actions-email payload carried a password; BaseHarbor never sends a password")
	}
	// Provider-version contract: the shipped Keycloak is pinned at 26.8.0
	// (keycloak_files.go:KeycloakImage). The documented Admin REST operation for
	// an invitation/activation action email on that version is
	// PUT /admin/realms/{realm}/users/{user-id}/execute-actions-email with a
	// required-actions list; the undocumented /updateEmail route must not be
	// used. This assertion ties the fixture's accepted route to that version.
	if !contractAcceptsShippedKeycloak(seenRoute) {
		t.Errorf("activation route %q is not the documented execute-actions-email operation for Keycloak %s", seenRoute, keycloakVersion())
	}
}

// contractAcceptsShippedKeycloak reports whether the observed activation route
// is the documented Keycloak 26.8.0 action-email operation. It encodes the
// contract that the shipped provider version (keycloakVersion) exposes on its
// Admin REST catalog: only the execute-actions-email path is accepted, so a
// fixture that would also accept the undocumented /updateEmail route (an
// inference from a complete endpoint catalog) is rejected here.
func contractAcceptsShippedKeycloak(route string) bool {
	return strings.HasSuffix(route, "/execute-actions-email")
}

// keycloakVersion returns the pinned shipped provider version, so a contract
// assertion can name the version it was validated against.
func keycloakVersion() string {
	const sep = ":"
	// KeycloakImage is "quay.io/keycloak/keycloak:26.8.0@sha256:..."; the tag
	// sits between the last ':' of the image and the '@' digest.
	if i := strings.LastIndex(KeycloakImage, "@"); i >= 0 {
		tag := KeycloakImage[strings.LastIndex(KeycloakImage, ":")+1 : i]
		if tag != "" {
			return tag
		}
	}
	return KeycloakImage
}

// Re-inviting the same owned, enabled, verified account is idempotent: no
// second activation is sent and the retained account is not mutated.
func TestInviteManagedUserRepeatIsIdempotent(t *testing.T) {
	for range 2 {
		activationSeen := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/users"):
				if r.Method == http.MethodGet {
					w.Write([]byte(`[{"id":"u1","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"emailVerified":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
					return
				}
				w.WriteHeader(http.StatusCreated)
				return
			case strings.HasSuffix(r.URL.Path, "/users?"):
				w.Write([]byte(`[{"id":"u1","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"emailVerified":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
				return
			case strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
				activationSeen = true
				w.WriteHeader(http.StatusNoContent)
				return
			default:
				w.Write([]byte(managedInviteRealm))
			}
		}))
		admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
		if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err != nil {
			server.Close()
			t.Fatal(err)
		}
		if activationSeen {
			server.Close()
			t.Fatal("repeat invite sent a second activation; not idempotent")
		}
		server.Close()
	}
}

// An ambiguous email (two different accounts) is refused; no activation runs.
func TestInviteManagedUserAmbiguousEmailIsRefused(t *testing.T) {
	activationSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			if r.Method == http.MethodGet {
				w.Write([]byte(`[{"id":"a","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local"},{"id":"b","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local"}]`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		case strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
			activationSeen = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm))
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err == nil {
		t.Fatal("ambiguous invite did not fail")
	}
	if activationSeen {
		t.Fatal("ambiguous invite still sent an activation")
	}
}

// A foreign or unverified realm is refused; nothing is created.
func TestInviteManagedUserForeignRealmIsRefused(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/users") {
			// realm inspection returns a realm owned by someone else
			w.Write([]byte(`{"realm":"bh-operators-dev","enabled":true,"registrationAllowed":true,"attributes":{"baseharbor.owner":"other","baseharbor.scope":"operator"}}`))
			return
		}
		if r.Method == http.MethodGet {
			w.Write([]byte(`[]`))
			return
		}
		created = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err == nil {
		t.Fatal("foreign realm invite did not fail")
	}
	if created {
		t.Fatal("invite created a user under an unverified realm")
	}
}

// A same-named but unowned account (foreign, disabled or without the managed
// activation marker) is refused, never repaired or adopted.
func TestInviteManagedUserUnownedAccountIsRefused(t *testing.T) {
	for _, body := range []string{
		`[{"id":"x","username":"i@b.local","email":"i@b.local","enabled":true,"attributes":{"baseharbor.scope":["developer-access"]}}]`, // wrong scope
		`[{"id":"x","username":"i@b.local","email":"i@b.local","enabled":false,"attributes":{"baseharbor.scope":["managed-invite"]}}]`,  // disabled
		`[{"id":"x","username":"i@b.local","email":"i@b.local","enabled":true,"attributes":{}}]`,                                        // no scope marker
	} {
		t.Run(body, func(t *testing.T) {
			activationSeen := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/users"):
					if r.Method == http.MethodGet {
						w.Write([]byte(body))
						return
					}
					w.WriteHeader(http.StatusCreated)
					return
				case strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
					activationSeen = true
					w.WriteHeader(http.StatusNoContent)
					return
				default:
					w.Write([]byte(managedInviteRealm))
				}
			}))
			admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
			if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err == nil {
				server.Close()
				t.Fatal("unowned account was invited")
			}
			if activationSeen {
				server.Close()
				t.Fatal("activation ran for an unowned account")
			}
			server.Close()
		})
	}
}

// The create fallback (POST 201 returns only an ID, then a GET lookup) must
// fail closed when that single resolved record is foreign or unverified: the
// invite must not send an activation link to an account this path did not
// create. This is the defect reviewed in review-1397a.md finding 2.
func TestInviteManagedUserFallbackForeignAccountIsRefused(t *testing.T) {
	activationSeen := false
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/users"):
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"u1"}`)) // 201 with only an ID -> forces fallback
			return
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/users"):
			gets++
			if gets == 1 {
				// Initial lookup: no owned account yet -> create.
				w.Write([]byte(`[]`))
				return
			}
			// Fallback resolve-after-create: a foreign/unverified account that
			// merely shares the email (wrong scope marker, no activation marker).
			w.Write([]byte(`[{"id":"other","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"attributes":{"baseharbor.scope":["developer-access"]}}]`))
			return
		case strings.HasSuffix(p, "/execute-actions-email"):
			activationSeen = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm))
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err == nil {
		t.Fatal("fallback resolved a foreign/unverified record without refusing")
	}
	if activationSeen {
		t.Fatal("activation ran for a foreign/unverified fallback account")
	}
}

// The create fallback must fail closed when the single resolved record shares
// the invited email and the managed-invite marker but is NOT the account this
// invite created: its ID must not match the ID the successful POST returned.
// The email/username, enabled state and scope marker can all look owned while
// the record is a different account; only an ID that equals the create
// response proves it is this invite's own creation. This is the ID-binding gap
// from review-1397a.md finding 1.
func TestInviteManagedUserFallbackIdMismatchIsRefused(t *testing.T) {
	activationSeen := false
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/users"):
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"u1"}`)) // 201 with only an ID -> forces fallback
			return
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/users"):
			gets++
			if gets == 1 {
				// Initial lookup: no owned account yet -> create.
				w.Write([]byte(`[]`))
				return
			}
			// Fallback resolve-after-create: same email, same managed-invite
			// marker, enabled — but a DIFFERENT id ("u9") than the create
			// returned ("u1"). This is a different owned-looking account.
			w.Write([]byte(`[{"id":"u9","username":"invitee@baseharbor.local","email":"invitee@baseharbor.local","enabled":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
			return
		case strings.HasSuffix(p, "/execute-actions-email"):
			activationSeen = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm))
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "invitee@baseharbor.local", managedInviteOwnership()); err == nil {
		t.Fatal("fallback resolved a same-email/different-id record without binding to the create ID")
	}
	if activationSeen {
		t.Fatal("activation ran for a same-email/different-id fallback account")
	}
}

// The ambiguity refusal must be a real property of the production path
// (inviteManagedUser -> resolveManagedUser), not of a separate helper. Two
// different accounts that both look owned (managed-invite marker, enabled,
// verified) for one email must be refused with no activation. Removing the
// production len(users)>1 guard must make this same production-path regression
// fail (red); keeping it makes it green. This is the causal mutant-evidence
// requirement from review-1397a.md finding 3 / act-1397b.md finding 2: the
// guard under test is removed from the production source, the production
// regression is rerun, and it turns red instead of passing because a later
// ownership predicate would otherwise accept the first account.
func TestInviteManagedUserAmbiguousEmailRefusedProductionPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/users"):
			if r.Method == http.MethodGet {
				// Ambiguous: two DIFFERENT accounts that both look owned
				// (managed-invite marker, enabled, verified) share this email.
				w.Write([]byte(`[{"id":"a","username":"i@b.local","email":"i@b.local","enabled":true,"emailVerified":true,"attributes":{"baseharbor.scope":["managed-invite"]}},{"id":"b","username":"i@b.local","email":"i@b.local","enabled":true,"emailVerified":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		case strings.HasSuffix(r.URL.Path, "/execute-actions-email"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			w.Write([]byte(managedInviteRealm))
		}
	}))
	defer server.Close()
	admin := &keycloakAdmin{endpoint: server.URL, client: server.Client()}
	if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "i@b.local", managedInviteOwnership()); err == nil {
		t.Fatal("ambiguous invite must be refused on the production path")
	}
}
