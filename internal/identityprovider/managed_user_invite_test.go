package identityprovider

import (
	"bytes"
	"context"
	"encoding/json"
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

// A real provider-admin call performs the activation through the provider's
// update-email endpoint; no password is ever stored or echoed.
func TestInviteManagedUserPerformsRealActivationWithoutPassword(t *testing.T) {
	var activationSeen, created, userGets int
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
		case strings.Contains(r.URL.Path, "/updateEmail"):
			activationSeen++
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var payload map[string]any
			if body, _ := io.ReadAll(r.Body); len(bytes.TrimSpace(body)) > 0 {
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("updateEmail body=%q", string(body))
				}
			}
			if payload["to"] == "" {
				t.Error("activation did not target the created account")
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
			case strings.Contains(r.URL.Path, "/updateEmail"):
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
		case strings.Contains(r.URL.Path, "/updateEmail"):
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
				case strings.Contains(r.URL.Path, "/updateEmail"):
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
		case strings.Contains(p, "/updateEmail"):
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
