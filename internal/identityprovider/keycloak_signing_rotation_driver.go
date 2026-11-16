package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/credential"
)

type preparedKeycloakSigningRotation struct {
	OldComponent    keycloakComponent `json:"old_component"`
	NewComponent    keycloakComponent `json:"new_component"`
	OldKid          string            `json:"old_kid"`
	NewKid          string            `json:"new_kid"`
	OldToken        string            `json:"old_token"`
	NewToken        string            `json:"new_token,omitempty"`
	ProbeClientID   string            `json:"probe_client_id"`
	ProbeClientUUID string            `json:"probe_client_uuid"`
	ProbeSecret     string            `json:"probe_secret"`
}

// RotateSigningKey performs overlap-safe realm signing-key rotation. The new
// key is published passively before it becomes active. The old key remains in
// JWKS and continues to verify old tokens until replacement signing has been
// observed and cryptographically verified.
func (d *KeycloakDriver) RotateSigningKey(ctx context.Context) error {
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
	admin, err := d.adminClient(ctx)
	if err != nil {
		return err
	}

	scopeDir := filepath.Join(d.instance.StateDir, "scopes", d.realm)
	journal := credential.FileRotationJournal{Path: filepath.Join(scopeDir, "signing-key-rotation.json")}
	prepared := credential.FilePreparedMaterialStore{Dir: filepath.Join(scopeDir, "prepared-signing")}
	const key = "realm-signing-key"

	phase, err := journal.Load(key)
	if err != nil {
		return err
	}
	if phase == "" {
		oldComponent, oldKid, err := admin.ensureManagedSigningProvider(ctx, d.realm)
		if err != nil {
			return err
		}
		probeSecret, err := randomIdentitySecret(32)
		if err != nil {
			return err
		}
		probeClientID := d.clientID + "-signing-rotation-probe"
		probeUUID, err := admin.ensureSigningRotationProbeClient(ctx, d.realm, probeClientID, probeSecret)
		if err != nil {
			return err
		}
		oldToken, err := admin.mintSigningProbeToken(ctx, d.realm, probeClientID, probeSecret)
		if err != nil {
			return err
		}
		if kid, err := jwtKid(oldToken); err != nil || kid != oldKid {
			return fmt.Errorf("Keycloak old signing probe token kid mismatch: got %q want %q: %w", kid, oldKid, err)
		}
		jwks, err := fetchKeycloakJWKS(ctx, d.instance.PublicHTTPClient, d.instance.EndpointBaseURL, d.realm)
		if err != nil {
			return err
		}
		if err := verifyRS256JWTWithJWKS(oldToken, jwks); err != nil {
			return fmt.Errorf("verify pre-rotation Keycloak token: %w", err)
		}

		suffix, err := randomIdentitySecret(16)
		if err != nil {
			return err
		}
		newPriority := componentPriority(oldComponent) - 1
		if newPriority < 1 {
			newPriority = 1
		}
		newComponent, err := admin.createManagedSigningProvider(ctx, d.realm, managedSigningPrefix+"rotation-"+suffix[:12], newPriority)
		if err != nil {
			return err
		}
		keys, err := admin.signingKeys(ctx, d.realm)
		if err != nil {
			return err
		}
		newKid := kidForSigningComponent(keys, newComponent.ID)
		if newKid == "" || newKid == oldKid {
			return errors.New("Keycloak replacement signing key was not published with a distinct kid")
		}
		jwks, err = fetchKeycloakJWKS(ctx, d.instance.PublicHTTPClient, d.instance.EndpointBaseURL, d.realm)
		if err != nil {
			return err
		}
		if !jwksContainsKid(jwks, oldKid) || !jwksContainsKid(jwks, newKid) {
			return errors.New("Keycloak JWKS did not publish old and replacement signing keys before promotion")
		}
		material := preparedKeycloakSigningRotation{
			OldComponent:    oldComponent,
			NewComponent:    newComponent,
			OldKid:          oldKid,
			NewKid:          newKid,
			OldToken:        oldToken,
			ProbeClientID:   probeClientID,
			ProbeClientUUID: probeUUID,
			ProbeSecret:     probeSecret,
		}
		if err := savePreparedKeycloakSigningRotation(prepared, key, material); err != nil {
			return err
		}
		if err := journal.Save(key, credential.RotationPrepared); err != nil {
			return err
		}
		phase = credential.RotationPrepared
	}

	material, err := loadPreparedKeycloakSigningRotation(prepared, key)
	if err != nil {
		return err
	}

	if phase == credential.RotationPrepared {
		priority := componentPriority(material.OldComponent) + 100
		if err := admin.updateSigningProviderPriority(ctx, d.realm, material.NewComponent, priority); err != nil {
			return err
		}
		material.NewComponent.Config["priority"] = []string{fmt.Sprintf("%d", priority)}
		keys, err := admin.signingKeys(ctx, d.realm)
		if err != nil {
			return err
		}
		if active := strings.TrimSpace(keys.Active["RS256"]); active != material.NewKid {
			return fmt.Errorf("Keycloak replacement signer did not become active: got %q want %q", active, material.NewKid)
		}
		if err := savePreparedKeycloakSigningRotation(prepared, key, material); err != nil {
			return err
		}
		if err := journal.Save(key, credential.RotationReconciled); err != nil {
			return err
		}
		phase = credential.RotationReconciled
	}

	if phase == credential.RotationReconciled {
		material, err = d.verifySigningRotationOverlap(ctx, admin, material)
		if err != nil {
			return err
		}
		if err := savePreparedKeycloakSigningRotation(prepared, key, material); err != nil {
			return err
		}
		if err := journal.Save(key, credential.RotationVerified); err != nil {
			return err
		}
		phase = credential.RotationVerified
	}

	if phase == credential.RotationVerified {
		if err := admin.deleteManagedSigningProvider(ctx, d.realm, material.OldComponent); err != nil {
			return err
		}
		jwks, err := fetchKeycloakJWKS(ctx, d.instance.PublicHTTPClient, d.instance.EndpointBaseURL, d.realm)
		if err != nil {
			return err
		}
		if jwksContainsKid(jwks, material.OldKid) {
			return errors.New("retired Keycloak signing key remains published in JWKS")
		}
		if !jwksContainsKid(jwks, material.NewKid) {
			return errors.New("replacement Keycloak signing key disappeared after retirement")
		}
		if err := verifyRS256JWTWithJWKS(material.OldToken, jwks); err == nil {
			return errors.New("token signed by retired Keycloak key still verifies against current JWKS")
		}
		if strings.TrimSpace(material.NewToken) == "" {
			return errors.New("replacement Keycloak verification token is missing")
		}
		if err := verifyRS256JWTWithJWKS(material.NewToken, jwks); err != nil {
			return fmt.Errorf("replacement Keycloak token failed after retirement: %w", err)
		}
		if err := admin.deleteSigningRotationProbeClient(ctx, d.realm, material.ProbeClientUUID); err != nil {
			return err
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

func (d *KeycloakDriver) verifySigningRotationOverlap(ctx context.Context, admin *keycloakAdmin, material preparedKeycloakSigningRotation) (preparedKeycloakSigningRotation, error) {
	newToken, err := admin.mintSigningProbeToken(ctx, d.realm, material.ProbeClientID, material.ProbeSecret)
	if err != nil {
		return material, err
	}
	newKid, err := jwtKid(newToken)
	if err != nil {
		return material, err
	}
	if newKid != material.NewKid {
		return material, fmt.Errorf("Keycloak replacement token kid = %q, want %q", newKid, material.NewKid)
	}
	jwks, err := fetchKeycloakJWKS(ctx, d.instance.PublicHTTPClient, d.instance.EndpointBaseURL, d.realm)
	if err != nil {
		return material, err
	}
	if !jwksContainsKid(jwks, material.OldKid) || !jwksContainsKid(jwks, material.NewKid) {
		return material, errors.New("Keycloak signing-key overlap disappeared before verification")
	}
	if err := verifyRS256JWTWithJWKS(material.OldToken, jwks); err != nil {
		return material, fmt.Errorf("old Keycloak token is not verifiable during signing-key overlap: %w", err)
	}
	if err := verifyRS256JWTWithJWKS(newToken, jwks); err != nil {
		return material, fmt.Errorf("new Keycloak token is not verifiable before retirement: %w", err)
	}
	material.NewToken = newToken
	return material, nil
}

func loadPreparedKeycloakSigningRotation(store credential.PreparedMaterialStore, key string) (preparedKeycloakSigningRotation, error) {
	data, err := store.Load(key)
	if err != nil {
		return preparedKeycloakSigningRotation{}, err
	}
	if len(data) == 0 {
		return preparedKeycloakSigningRotation{}, errors.New("prepared Keycloak signing-key rotation state is missing")
	}
	var material preparedKeycloakSigningRotation
	if err := json.Unmarshal(data, &material); err != nil {
		return preparedKeycloakSigningRotation{}, err
	}
	if material.OldComponent.ID == "" || material.NewComponent.ID == "" ||
		material.OldKid == "" || material.NewKid == "" || material.OldKid == material.NewKid ||
		material.OldToken == "" || material.ProbeClientID == "" ||
		material.ProbeClientUUID == "" || material.ProbeSecret == "" {
		return preparedKeycloakSigningRotation{}, errors.New("prepared Keycloak signing-key rotation state is invalid")
	}
	return material, nil
}

func savePreparedKeycloakSigningRotation(store credential.PreparedMaterialStore, key string, material preparedKeycloakSigningRotation) error {
	data, err := json.Marshal(material)
	if err != nil {
		return err
	}
	return store.Save(key, data)
}

func signingRotationStateExists(instance KeycloakInstance, realm string) bool {
	path := filepath.Join(instance.StateDir, "scopes", realm, "signing-key-rotation.json")
	_, err := os.Stat(path)
	return err == nil
}
