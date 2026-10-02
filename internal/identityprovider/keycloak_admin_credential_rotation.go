package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/credential"
)

type preparedKeycloakAdminCredentialRotation struct {
	OldUsername string `json:"old_username"`
	OldPassword string `json:"old_password"`
	NewUsername string `json:"new_username"`
	NewPassword string `json:"new_password"`
}

func (d *KeycloakDriver) RotateAdminCredential(ctx context.Context) error {
	if d.realization == nil {
		return errors.New("Keycloak provider realization is required")
	}
	if strings.TrimSpace(d.instance.StateDir) == "" {
		instance, err := d.realization.Existing(ctx)
		if err != nil {
			return err
		}
		d.instance = instance
	}
	scopeDir := filepath.Join(d.instance.StateDir, "admin-credential-rotation")
	journal := credential.FileRotationJournal{Path: filepath.Join(scopeDir, "journal.json")}
	prepared := credential.FilePreparedMaterialStore{Dir: filepath.Join(scopeDir, "prepared")}
	const key = "keycloak-admin"

	phase, err := journal.Load(key)
	if err != nil {
		return err
	}
	if phase == "" {
		oldUser := strings.TrimSpace(d.instance.AdminUsername)
		oldPassword := strings.TrimSpace(d.instance.AdminPassword)
		if oldUser == "" || oldPassword == "" {
			return errors.New("Keycloak admin credential is incomplete")
		}
		suffix, err := randomIdentitySecret(8)
		if err != nil {
			return err
		}
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
		newPassword, err := randomIdentitySecret(32)
		if err != nil {
			return err
		}
		material := preparedKeycloakAdminCredentialRotation{
			OldUsername: oldUser,
			OldPassword: oldPassword,
			NewUsername: "baseharbor-admin-r-" + suffix,
			NewPassword: newPassword,
		}

		oldAdmin := &keycloakAdmin{
			endpoint: strings.TrimRight(d.instance.EndpointBaseURL, "/"),
			client:   d.instance.AdminHTTPClient,
			user:     material.OldUsername,
			password: material.OldPassword,
		}
		if err := oldAdmin.login(ctx); err != nil {
			return fmt.Errorf("authenticate current Keycloak admin before rotation: %w", err)
		}
		if err := oldAdmin.reconcileUser(ctx, "master", material.NewUsername, material.NewPassword, true); err != nil {
			return fmt.Errorf("prepare replacement Keycloak admin: %w", err)
		}
		if err := verifyKeycloakAdminCredential(ctx, d.instance, material.NewUsername, material.NewPassword); err != nil {
			_ = deleteKeycloakUserByUsername(ctx, oldAdmin, "master", material.NewUsername)
			return fmt.Errorf("verify prepared Keycloak admin: %w", err)
		}
		data, err := json.Marshal(material)
		if err != nil {
			return err
		}
		if err := prepared.Save(key, data); err != nil {
			return err
		}
		if err := journal.Save(key, credential.RotationPrepared); err != nil {
			return err
		}
		phase = credential.RotationPrepared
	}

	material, err := loadPreparedKeycloakAdminCredentialRotation(prepared, key)
	if err != nil {
		return err
	}

	if phase == credential.RotationPrepared {
		if err := replaceKeycloakAdminState(d.instance.StateDir, material.NewUsername, material.NewPassword); err != nil {
			return fmt.Errorf("project replacement Keycloak admin credential: %w", err)
		}
		d.instance.AdminUsername = material.NewUsername
		d.instance.AdminPassword = material.NewPassword
		if err := journal.Save(key, credential.RotationReconciled); err != nil {
			return err
		}
		phase = credential.RotationReconciled
	}

	if phase == credential.RotationReconciled {
		if err := verifyKeycloakAdminCredential(ctx, d.instance, material.NewUsername, material.NewPassword); err != nil {
			_ = replaceKeycloakAdminState(d.instance.StateDir, material.OldUsername, material.OldPassword)
			d.instance.AdminUsername = material.OldUsername
			d.instance.AdminPassword = material.OldPassword
			_ = journal.Save(key, credential.RotationPrepared)
			return fmt.Errorf("verify replacement Keycloak admin after projection: %w", err)
		}
		if err := journal.Save(key, credential.RotationVerified); err != nil {
			return err
		}
		phase = credential.RotationVerified
	}

	if phase == credential.RotationVerified {
		newAdmin := &keycloakAdmin{
			endpoint: strings.TrimRight(d.instance.EndpointBaseURL, "/"),
			client:   d.instance.AdminHTTPClient,
			user:     material.NewUsername,
			password: material.NewPassword,
		}
		if err := newAdmin.login(ctx); err != nil {
			return fmt.Errorf("authenticate replacement Keycloak admin before retirement: %w", err)
		}
		if err := deleteKeycloakUserByUsername(ctx, newAdmin, "master", material.OldUsername); err != nil {
			return fmt.Errorf("retire previous Keycloak admin: %w", err)
		}
		if err := verifyKeycloakAdminCredential(ctx, d.instance, material.OldUsername, material.OldPassword); err == nil {
			return errors.New("retired Keycloak admin credential is still accepted")
		}
		if err := verifyKeycloakAdminCredential(ctx, d.instance, material.NewUsername, material.NewPassword); err != nil {
			return fmt.Errorf("replacement Keycloak admin failed after retirement: %w", err)
		}
		if err := journal.Save(key, credential.RotationRetired); err != nil {
			return err
		}
		phase = credential.RotationRetired
	}

	if phase == credential.RotationRetired {
		if err := journal.Clear(key); err != nil {
			return err
		}
		if err := prepared.Clear(key); err != nil {
			return err
		}
	}
	return nil
}

func verifyKeycloakAdminCredential(ctx context.Context, instance KeycloakInstance, username, password string) error {
	if instance.AdminHTTPClient == nil {
		return errors.New("Keycloak realization returned no admin HTTP client")
	}
	admin := &keycloakAdmin{
		endpoint: strings.TrimRight(instance.EndpointBaseURL, "/"),
		client:   instance.AdminHTTPClient,
		user:     strings.TrimSpace(username),
		password: strings.TrimSpace(password),
	}
	if err := admin.login(ctx); err != nil {
		return err
	}
	status, body, err := admin.do(ctx, http.MethodGet, "/admin/serverinfo", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("Keycloak replacement admin lacks management access: HTTP %d: %s", status, body)
	}
	return nil
}

func deleteKeycloakUserByUsername(ctx context.Context, admin *keycloakAdmin, realm, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("Keycloak username is required")
	}
	query := url.Values{}
	query.Set("username", username)
	query.Set("exact", "true")
	status, body, err := admin.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/users?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("resolve Keycloak user %q for retirement: HTTP %d: %s", username, status, body)
	}
	var users []keycloakUser
	if err := json.Unmarshal([]byte(body), &users); err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}
	if len(users) != 1 || strings.TrimSpace(users[0].ID) == "" {
		return fmt.Errorf("Keycloak user %q is ambiguous", username)
	}
	status, body, err = admin.do(ctx, http.MethodDelete, "/admin/realms/"+url.PathEscape(realm)+"/users/"+url.PathEscape(users[0].ID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("delete Keycloak user %q: HTTP %d: %s", username, status, body)
	}
	return nil
}

func replaceKeycloakAdminState(stateDir, username, password string) error {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return errors.New("Keycloak state directory is required")
	}
	envPath := filepath.Join(stateDir, "runtime.env")
	values, err := readProtectedEnv(envPath)
	if err != nil {
		return err
	}
	values["BASEHARBOR_KEYCLOAK_ADMIN_USER"] = strings.TrimSpace(username)
	values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"] = strings.TrimSpace(password)
	return writeProtectedEnv(envPath, values)
}

func loadPreparedKeycloakAdminCredentialRotation(store credential.PreparedMaterialStore, key string) (preparedKeycloakAdminCredentialRotation, error) {
	data, err := store.Load(key)
	if err != nil {
		return preparedKeycloakAdminCredentialRotation{}, err
	}
	if len(data) == 0 {
		return preparedKeycloakAdminCredentialRotation{}, errors.New("prepared Keycloak admin rotation state is missing")
	}
	var material preparedKeycloakAdminCredentialRotation
	if err := json.Unmarshal(data, &material); err != nil {
		return preparedKeycloakAdminCredentialRotation{}, err
	}
	material.OldUsername = strings.TrimSpace(material.OldUsername)
	material.OldPassword = strings.TrimSpace(material.OldPassword)
	material.NewUsername = strings.TrimSpace(material.NewUsername)
	material.NewPassword = strings.TrimSpace(material.NewPassword)
	if material.OldUsername == "" || material.OldPassword == "" ||
		material.NewUsername == "" || material.NewPassword == "" ||
		material.OldUsername == material.NewUsername || material.OldPassword == material.NewPassword {
		return preparedKeycloakAdminCredentialRotation{}, errors.New("prepared Keycloak admin rotation state is invalid")
	}
	return material, nil
}

func keycloakAdminRotationStateExists(instance KeycloakInstance) bool {
	_, err := os.Stat(filepath.Join(instance.StateDir, "admin-credential-rotation", "journal.json"))
	return err == nil
}
