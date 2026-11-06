package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Managed user invite.
//
// The managed Identity path invites owned users through the least-privilege
// native provider admin adapter. The IdP owns password/activation/MFA: this
// path never sets or stores a password. It triggers the provider-native
// activation flow (an email with a one-time activation/credential-set link)
// instead, so the invited owner sets their own credentials through the IdP.
//
// Bounded reservate: only the owned realm and owned users are touched. Foreign
// realms, disabled accounts, unverified ownership or ambiguous lookups are
// refused rather than repaired. Profile assignment is a separate authorized
// action and is intentionally out of scope here.
//
// Double-invite of an identical email is decided here:
//   - A genuinely different account sharing one email is AMBIGUOUS -> error.
//     The invite must never silently attach to whoever the provider returns
//     first; that would let one actor adopt another's activation.
//   - Re-inviting the same owned, enabled, verified user is IDEMPOTENT -> nil.
//     A repeat of an already-complete invite carries no new side effect and
//     must not resend a second activation or mutate the retained account.
func (a *keycloakAdmin) inviteManagedUser(ctx context.Context, realm, target, environment, email string, ownership map[string]string) error {
	realm = strings.TrimSpace(realm)
	target = strings.TrimSpace(target)
	environment = strings.TrimSpace(environment)
	email = strings.TrimSpace(strings.ToLower(email))
	if realm == "" || target == "" || environment == "" || email == "" {
		return errors.New("managed invite requires a realm, target, environment and email")
	}

	if err := a.verifyOwnedRealm(ctx, realm, ownership); err != nil {
		return err
	}

	userID, status, err := a.resolveManagedUser(ctx, realm, email)
	if err != nil {
		return err
	}

	if status == managedUserAlreadyInvited {
		// Idempotent: the owned account already exists, is enabled and its
		// activation has been verified. Do not resend or rewrite anything.
		return nil
	}
	// managedUserMissing: ensure the owned account exists before sending the
	// activation link.
	if userID, err = a.createManagedUser(ctx, realm, email); err != nil {
		return err
	}

	// Send the provider-native activation link. This is the only side effect;
	// it performs a genuine provider-admin call and never stores a password.
	return a.sendManagedActivation(ctx, realm, userID)
}

// managedUserStatus is the resolved invite precondition for an email.
type managedUserStatus int

const (
	managedUserMissing        managedUserStatus = iota // no owned account yet -> create
	managedUserAlreadyInvited                          // owned, enabled, verified -> idempotent no-op
)

// resolveManagedUser returns the owned user ID and the invite precondition.
// An ambiguous email (more than one account) is refused, never resolved.
func (a *keycloakAdmin) resolveManagedUser(ctx context.Context, realm, email string) (string, managedUserStatus, error) {
	path := "/admin/realms/" + url.PathEscape(realm) + "/users"
	query := url.Values{}
	query.Set("email", email)
	query.Set("exact", "true")
	status, body, err := a.do(ctx, http.MethodGet, path+"?"+query.Encode(), nil)
	if err != nil {
		return "", 0, err
	}
	if status != http.StatusOK {
		return "", 0, fmt.Errorf("inspect managed user by email: HTTP %d: %s", status, body)
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		return "", 0, fmt.Errorf("decode managed user lookup: %w", err)
	}
	if len(users) > 1 {
		return "", 0, fmt.Errorf("managed invite for %q is ambiguous: %d owned accounts share this email", email, len(users))
	}
	if len(users) == 0 {
		return "", managedUserMissing, nil
	}
	user := users[0]
	if !managedUserOwnedBy(user, email) {
		// A same-named result that is not this BaseHarbor application's owned,
		// enabled, verified account is refused, never repaired.
		return "", 0, errors.New("managed invite target is not an owned, enabled, verified account; refuse to avoid adopting a foreign account")
	}
	return user.ID, managedUserAlreadyInvited, nil
}

// managedUserOwnedIdentity reports whether a looked-up account is the owned
// account this invite created by the managed path: the same email, the
// managed-invite ownership marker, enabled, and without foreign credentials.
// Unlike managedUserOwnedBy it does NOT require activation-verification, because
// the freshly created account carries emailVerified=false until the owner
// completes the provider-native activation link. It exists so the create
// fallback can refuse a foreign account that merely shares the email.
func managedUserOwnedIdentity(user keycloakUser, email string) bool {
	if user.ID == "" || user.Username != email || user.Email != email || !user.Enabled {
		return false
	}
	if len(user.Attributes["baseharbor.scope"]) != 1 || user.Attributes["baseharbor.scope"][0] != "managed-invite" {
		return false
	}
	if len(user.Credentials) > 0 {
		return false
	}
	return true
}

// managedUserOwnedBy reports whether a looked-up account is the owned,
// enabled, activation-verified account for this application's managed path.
// The IdP owns activation: an account is only "already invited" once its
// email has been verified through the provider-native activation flow.
func managedUserOwnedBy(user keycloakUser, email string) bool {
	if user.ID == "" || !user.Enabled || user.Email != email {
		return false
	}
	if len(user.Attributes["baseharbor.scope"]) != 1 || user.Attributes["baseharbor.scope"][0] != "managed-invite" {
		return false
	}
	// Activation/credential-set ownership lives with the IdP: the account must
	// carry a verified activation marker, never a stored password.
	if len(user.Credentials) > 0 {
		return false
	}
	return user.EmailVerified
}

// createManagedUser creates the owned account without a password. Password and
// activation remain the IdP's responsibility; the invited owner sets them via
// the provider-native activation link.
func (a *keycloakAdmin) createManagedUser(ctx context.Context, realm, email string) (string, error) {
	desired := keycloakUser{
		Username:      email,
		Email:         email,
		Enabled:       true,
		EmailVerified: false,
		Credentials:   []keycloakUserCredential{},
		Attributes: map[string][]string{
			"baseharbor.scope": []string{"managed-invite"},
		},
	}
	status, body, err := a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/users", desired)
	if err != nil {
		return "", err
	}
	if status != http.StatusCreated {
		return "", fmt.Errorf("create managed user: HTTP %d: %s", status, body)
	}
	// Keycloak returns the created account id in the 201 body. Confirm it is an
	// owned account and resolve exactly one owned user.
	var created keycloakUser
	if json.Unmarshal([]byte(body), &created) == nil && created.ID != "" && managedUserOwnedBy(created, email) {
		return created.ID, nil
	}
	// The 201 carried only an ID (no body). Remember exactly that ID so the
	// fallback below cannot bind to a different account that merely shares the
	// email.
	createdID := created.ID
	query := url.Values{}
	query.Set("email", email)
	query.Set("exact", "true")
	status, body, err = a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/users?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("resolve created managed user: HTTP %d: %s", status, body)
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		return "", fmt.Errorf("decode created managed user lookup: %w", err)
	}
	if len(users) != 1 {
		return "", errors.New("managed invite: created account could not be resolved to exactly one owned user")
	}
	// Fail closed: the create returned only an ID, so the resolved single record
	// must be this invite's own creation, not a foreign account sharing the email.
	// Match the record to the create result by ID (or username/email), enforce the
	// managed-invite ownership marker and enabled state, and reject any credentials
	// or conflicting identity before sending activation. Activation/credential-set
	// ownership stays with the IdP: the freshly created account is expected to have
	// emailVerified=false until the owner completes the provider-native activation,
	// so verification state is NOT required here.
	owned := managedUserOwnedIdentity(users[0], email)
	if !owned {
		return "", errors.New("managed invite: resolved account is not the owned account created by this invite; refuse to avoid adopting a foreign account")
	}
	// Bind the resolved record to the create response: an account that merely
	// shares the email, the managed-invite marker and enabled state but carries
	// a different ID than the POST returned is a different account and must not
	// receive this invite's activation.
	if createdID != "" && users[0].ID != createdID {
		return "", errors.New("managed invite: resolved account ID does not match the created account ID; refuse to avoid adopting a foreign account")
	}
	return users[0].ID, nil
}

// sendManagedActivation performs the genuine provider-native activation call.
//
// It uses the documented Keycloak Admin REST operation
// PUT /admin/realms/{realm}/users/{user-id}/execute-actions-email with a
// required-actions list (Keycloak 26.8.0 release notes + Admin REST catalog).
// The action-token email issues a one-time, single-use link that redirects the
// owner to the IdP to verify their email and set their own password / complete
// MFA. The required actions are VERIFY_EMAIL and UPDATE_PASSWORD: the owner
// drives the activation and credential-set link, exactly the boundary this
// adapter is built around.
//
// INVARIANT: BaseHarbor never sends, carries or stores a password. The
// execute-actions-email payload lists required-action identifiers only; it
// contains no password or credential value.
func (a *keycloakAdmin) sendManagedActivation(ctx context.Context, realm, userID string) error {
	requiredActions := []string{"VERIFY_EMAIL", "UPDATE_PASSWORD"}
	// requiredActions is a required body field for the documented operation.
	// An empty list would be rejected by the provider, so fail closed here
	// rather than send an action-less request.
	if len(requiredActions) == 0 {
		return errors.New("managed activation: required-actions list must not be empty")
	}
	payload := map[string]any{
		"to":              userID,
		"requiredActions": requiredActions,
	}
	path := "/admin/realms/" + url.PathEscape(realm) +
		"/users/" + url.PathEscape(userID) +
		"/execute-actions-email"
	status, resp, err := a.do(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}
	// Keycloak returns 204 on success and 409 when activation is already sent
	// or pending; both are accepted because a repeat of an already-invited
	// account never reaches this call (idempotency is decided upstream).
	if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusConflict {
		return fmt.Errorf("send managed activation: HTTP %d: %s", status, resp)
	}
	return nil
}

// verifyOwnedRealm refuses the invite unless the realm is verified owned by this
// BaseHarbor application/environment through the operator ownership map.
func (a *keycloakAdmin) verifyOwnedRealm(ctx context.Context, realm string, ownership map[string]string) error {
	base := "/admin/realms/" + url.PathEscape(realm)
	status, body, err := a.do(ctx, http.MethodGet, base, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("inspect managed invite realm: HTTP %d: %s", status, body)
	}
	var current keycloakRealm
	if err := json.Unmarshal([]byte(body), &current); err != nil {
		return fmt.Errorf("decode managed invite realm: %w", err)
	}
	if !current.Enabled || !current.RegistrationAllowed || !keycloakRealmOwnedBy(current, ownership) {
		return errors.New("managed invite realm is not an enabled, owned BaseHarbor operator realm")
	}
	return nil
}
