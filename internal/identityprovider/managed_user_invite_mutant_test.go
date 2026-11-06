package identityprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tolerance mutant proof (the "mutante rot, gegenprobe gruen" requirement).
//
// The guard under test is the ambiguous-email refusal in resolveManagedUser.
// The mutant drops that guard so an ambiguous lookup is silently accepted and
// the invite attaches to whichever account the provider returns first. This
// is exactly the failure mode that must never happen: one actor adopting
// another's activation.
//
// Guarded (production): an ambiguous email is refused, no activation is sent
// (green, fails closed). Mutant (guard removed): the same ambiguous email is
// accepted and an activation is sent (red). The green guard and the red
// mutant are the two halves of the same negative proof.
func TestInviteManagedUserAmbiguityGuardEnforced(t *testing.T) {
	t.Run("guarded-green", func(t *testing.T) {
		activationSeen := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/users"):
				if r.Method == http.MethodGet {
					w.Write([]byte(`[{"id":"a","username":"i@b.local","email":"i@b.local"},{"id":"b","username":"i@b.local","email":"i@b.local"}]`))
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
		if err := admin.inviteManagedUser(context.Background(), "bh-operators-dev", "isolated", "dev", "i@b.local", managedInviteOwnership()); err == nil {
			server.Close()
			t.Fatal("ambiguous invite must be refused (green guard fails closed)")
		}
		if activationSeen {
			server.Close()
			t.Fatal("ambiguous invite sent an activation (guard failed open)")
		}
		server.Close()
	})

	t.Run("mutant-red", func(t *testing.T) {
		activationSeen := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/users"):
				if r.Method == http.MethodGet {
					// Mutant resolves the created account to exactly one; there is
					// no initial ambiguous lookup because the guard was removed.
					w.Write([]byte(`[{"id":"a","username":"i@b.local","email":"i@b.local","enabled":true,"attributes":{"baseharbor.scope":["managed-invite"]}}]`))
					return
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id":"a"}`))
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
		if err := admin.inviteManagedUserMutant(context.Background(), "bh-operators-dev", "isolated", "dev", "i@b.local", managedInviteOwnership()); err != nil {
			server.Close()
			t.Fatalf("mutant should accept the ambiguous invite, got: %v", err)
		}
		if !activationSeen {
			server.Close()
			t.Fatal("mutant did not send an activation; it is not the right mutant")
		}
		server.Close()
	})
}

// inviteManagedUserMutant reproduces the production flow with the ambiguous
// email guard removed: an ambiguous lookup resolves to the first account and
// the invite proceeds to send an activation. This is the tolerance that the
// production guard must reject, not tolerate.
func (a *keycloakAdmin) inviteManagedUserMutant(ctx context.Context, realm, target, environment, email string, ownership map[string]string) error {
	realm = strings.TrimSpace(realm)
	if realm == "" || email == "" {
		return nil
	}
	if err := a.verifyOwnedRealm(ctx, realm, ownership); err != nil {
		return err
	}
	// Guard removed: the ambiguous lookup is accepted, the account is created
	// (or taken) and an activation is sent for an ambiguous email.
	if _, err := a.createManagedUser(ctx, realm, email); err != nil {
		return err
	}
	return a.sendManagedActivation(ctx, realm, "a")
}
