package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// ManagedOperatorUser is the directory-safe projection of a BaseHarbor-owned
// operator account. It never contains credentials or session tokens.
type ManagedOperatorUser struct {
	ID          string
	Login       string
	DisplayName string
	Email       string
	Enabled     bool
}

// InviteManagedOperatorUser creates a BaseHarbor operator account and sets a
// temporary password supplied by the Core owner. Email is contact metadata;
// account creation does not depend on SMTP or email verification.
func InviteManagedOperatorUser(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, target, environment, login, displayName, email, initialPassword string) (ManagedOperatorUser, error) {
	if strings.TrimSpace(initialPassword) == "" {
		return ManagedOperatorUser{}, errors.New("an initial password is required")
	}
	if err := ValidateManagedOperatorUser(login, displayName, email); err != nil {
		return ManagedOperatorUser{}, err
	}
	if _, err := EnsureManagedOperatorOIDC(ctx, runtime, issuer, dataDir, namespace, target, environment); err != nil {
		return ManagedOperatorUser{}, err
	}
	_, admin, realm, ownership, err := operatorUserAdmin(ctx, dataDir, namespace, target, environment)
	if err != nil {
		return ManagedOperatorUser{}, err
	}
	user, err := admin.inviteManagedOperatorUser(ctx, realm, login, displayName, email, initialPassword, ownership)
	if err != nil {
		return ManagedOperatorUser{}, err
	}
	return user, nil
}

// ListManagedOperatorUsers lists only users created and owned by the
// workstation-management flow in the selected BaseHarbor operator realm.
func ListManagedOperatorUsers(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, target, environment string) ([]ManagedOperatorUser, error) {
	_, admin, realm, ownership, err := operatorUserAdmin(ctx, dataDir, namespace, target, environment)
	if err != nil {
		return nil, err
	}
	users, err := admin.managedOperatorUsers(ctx, realm, ownership)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// DisableManagedOperatorUser disables one explicitly owned account and ends
// its live Keycloak sessions. Foreign or ambiguously resolved users are never
// modified.
func DisableManagedOperatorUser(ctx context.Context, dataDir, namespace, target, environment, login string) error {
	if strings.TrimSpace(login) != login || login == "" {
		return errors.New("a managed operator login is required")
	}
	_, admin, realm, ownership, err := operatorUserAdmin(ctx, dataDir, namespace, target, environment)
	if err != nil {
		return err
	}
	user, err := admin.resolveOwnedOperatorUser(ctx, realm, login, ownership)
	if err != nil {
		return err
	}
	if user.Enabled {
		status, body, err := admin.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(user.ID), map[string]any{"enabled": false})
		if err != nil {
			return err
		}
		if status != http.StatusNoContent && status != http.StatusOK {
			return fmt.Errorf("disable managed operator account: HTTP %d: %s", status, body)
		}
	}
	status, body, err := admin.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(user.ID)+"/logout", map[string]any{})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("end managed operator sessions: HTTP %d: %s", status, body)
	}
	return nil
}

func operatorUserAdmin(ctx context.Context, dataDir, namespace, target, environment string) (KeycloakFiles, *keycloakAdmin, string, map[string]string, error) {
	app := application.Manifest{Version: application.CurrentVersion, Name: "operator-access", Environment: strings.TrimSpace(environment), Services: application.Services{Identity: true}}
	files, err := ExistingKeycloakFilesAt(app, dataDir, namespace)
	if err != nil {
		return KeycloakFiles{}, nil, "", nil, err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return KeycloakFiles{}, nil, "", nil, err
	}
	userOwnership := map[string]string{
		"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator-workstation",
		"baseharbor.target": strings.TrimSpace(target), "baseharbor.environment": strings.TrimSpace(environment),
	}
	return files, admin, operatorRealm(environment), userOwnership, nil
}

func (a *keycloakAdmin) inviteManagedOperatorUser(ctx context.Context, realm, login, displayName, email, initialPassword string, ownership map[string]string) (ManagedOperatorUser, error) {
	realmOwnership := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": ownership["baseharbor.target"], "baseharbor.environment": ownership["baseharbor.environment"]}
	if err := a.verifyOwnedRealm(ctx, realm, realmOwnership); err != nil {
		return ManagedOperatorUser{}, err
	}
	if err := a.ensureWorkstationUserAttributes(ctx, realm, ownership); err != nil {
		return ManagedOperatorUser{}, err
	}
	path := "/admin/realms/" + url.PathEscape(realm) + "/users"
	query := url.Values{"username": {login}, "exact": {"true"}}
	status, body, err := a.do(ctx, http.MethodGet, path+"?"+query.Encode(), nil)
	if err != nil {
		return ManagedOperatorUser{}, err
	}
	if status != http.StatusOK {
		return ManagedOperatorUser{}, fmt.Errorf("inspect managed operator user: HTTP %d: %s", status, body)
	}
	var found []keycloakUser
	if err := json.Unmarshal([]byte(body), &found); err != nil || len(found) > 1 {
		return ManagedOperatorUser{}, errors.New("managed operator login is ambiguous or unreadable")
	}
	var user keycloakUser
	if len(found) == 1 {
		user = found[0]
		if !managedWorkstationUserOwned(user, login, email, ownership) {
			return ManagedOperatorUser{}, errors.New("login already belongs to an account not owned by this BaseHarbor onboarding")
		}
		if values := user.Attributes["baseharbor.display_name"]; len(values) != 1 || values[0] != displayName {
			return ManagedOperatorUser{}, errors.New("managed operator login already exists with different owner details")
		}
		if !user.Enabled {
			return ManagedOperatorUser{}, errors.New("managed operator account is disabled; use explicit recovery instead of re-onboarding")
		}
	} else {
		attributes := make(map[string][]string, len(ownership)+2)
		for key, value := range ownership {
			attributes[key] = []string{value}
		}
		attributes["baseharbor.display_name"] = []string{displayName}
		firstName, lastName := splitOperatorName(displayName)
		user = keycloakUser{Username: login, FirstName: firstName, LastName: lastName, Email: strings.ToLower(email), Enabled: true, EmailVerified: false, Attributes: attributes, Credentials: []keycloakUserCredential{}}
		status, body, err = a.do(ctx, http.MethodPost, path, user)
		if err != nil {
			return ManagedOperatorUser{}, err
		}
		if status != http.StatusCreated {
			return ManagedOperatorUser{}, fmt.Errorf("create managed operator account: HTTP %d: %s", status, body)
		}
		verifyQuery := url.Values{"username": {login}, "exact": {"true"}, "email": {strings.ToLower(email)}}
		status, body, err = a.do(ctx, http.MethodGet, path+"?"+verifyQuery.Encode(), nil)
		if err != nil || status != http.StatusOK || json.Unmarshal([]byte(body), &found) != nil || len(found) != 1 || !managedWorkstationUserOwned(found[0], login, email, ownership) {
			return ManagedOperatorUser{}, errors.New("created operator account could not be verified against its exact login and owner metadata")
		}
		user = found[0]
	}
	credential := keycloakUserCredential{Type: "password", Value: initialPassword, Temporary: true}
	status, body, err = a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(user.ID)+"/reset-password", credential)
	if err != nil {
		return ManagedOperatorUser{}, err
	}
	if status != http.StatusNoContent {
		return ManagedOperatorUser{}, fmt.Errorf("set initial managed operator password: HTTP %d: %s", status, body)
	}
	return managedOperatorUserProjection(user), nil
}

func splitOperatorName(fullName string) (string, string) {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return "", ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func (a *keycloakAdmin) resolveOwnedOperatorUser(ctx context.Context, realm, login string, ownership map[string]string) (keycloakUser, error) {
	query := url.Values{"username": {login}, "exact": {"true"}}
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/users?"+query.Encode(), nil)
	if err != nil || status != http.StatusOK {
		return keycloakUser{}, errors.New("managed operator account lookup failed")
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil || len(users) != 1 || !managedWorkstationUserOwned(users[0], login, users[0].Email, ownership) {
		return keycloakUser{}, errors.New("operator login does not resolve to exactly one owned BaseHarbor account")
	}
	return users[0], nil
}

func (a *keycloakAdmin) managedOperatorUsers(ctx context.Context, realm string, ownership map[string]string) ([]ManagedOperatorUser, error) {
	realmOwnership := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "operator", "baseharbor.target": ownership["baseharbor.target"], "baseharbor.environment": ownership["baseharbor.environment"]}
	if err := a.verifyOwnedRealm(ctx, realm, realmOwnership); err != nil {
		return nil, err
	}
	query := url.Values{"max": {"1000"}, "first": {"0"}}
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/users?"+query.Encode(), nil)
	if err != nil || status != http.StatusOK {
		return nil, errors.New("managed operator user inventory is unavailable")
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		return nil, err
	}
	var result []ManagedOperatorUser
	for _, user := range users {
		if managedWorkstationUserOwned(user, user.Username, user.Email, ownership) {
			result = append(result, managedOperatorUserProjection(user))
		}
	}
	return result, nil
}

func managedWorkstationUserOwned(user keycloakUser, login, email string, ownership map[string]string) bool {
	if user.ID == "" || user.Username != login || !strings.EqualFold(user.Email, email) {
		return false
	}
	for key, value := range ownership {
		if len(user.Attributes[key]) != 1 || user.Attributes[key][0] != value {
			return false
		}
	}
	return len(user.Attributes["baseharbor.scope"]) == 1 && user.Attributes["baseharbor.scope"][0] == "operator-workstation"
}

func managedOperatorUserProjection(user keycloakUser) ManagedOperatorUser {
	name := ""
	if values := user.Attributes["baseharbor.display_name"]; len(values) == 1 {
		name = values[0]
	}
	return ManagedOperatorUser{ID: user.ID, Login: user.Username, DisplayName: name, Email: user.Email, Enabled: user.Enabled}
}

func ValidateManagedOperatorUser(login, displayName, email string) error {
	validLogin := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@_-]{0,127}$`)
	parsedEmail, emailErr := mail.ParseAddress(email)
	if strings.TrimSpace(login) != login || !validLogin.MatchString(login) ||
		strings.TrimSpace(displayName) != displayName || displayName == "" || len(displayName) > 200 ||
		strings.TrimSpace(email) != email || emailErr != nil || parsedEmail.Address != email || len(email) > 254 {
		return errors.New("managed operator requires a valid login, person's full name and email")
	}
	return nil
}
