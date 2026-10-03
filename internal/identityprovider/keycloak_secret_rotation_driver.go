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
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/credential"
)

type preparedKeycloakClientSecretRotation struct {
	Current string `json:"current"`
	Rotated string `json:"rotated"`
}

func (d *KeycloakDriver) RotateClientSecret(ctx context.Context) error {
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
	if err := admin.ensureClientSecretRotationPolicy(ctx, d.realm); err != nil {
		return err
	}
	clientUUID, err := admin.resolveClientUUID(ctx, d.realm, d.clientID)
	if err != nil {
		return err
	}

	scopeDir := filepath.Join(d.instance.StateDir, "scopes", d.realm)
	journal := credential.FileRotationJournal{Path: filepath.Join(scopeDir, "client-secret-rotation.json")}
	prepared := credential.FilePreparedMaterialStore{Dir: filepath.Join(scopeDir, "prepared")}
	const key = "oidc-client-secret"

	phase, err := journal.Load(key)
	if err != nil {
		return err
	}
	if phase == "" {
		oldLocal, err := readKeycloakClientSecretFile(keycloakClientSecretPath(d.instance, d.realm))
		if err != nil {
			return err
		}
		oldRemote, err := admin.currentClientSecret(ctx, d.realm, clientUUID)
		if err != nil {
			return err
		}
		if oldRemote != oldLocal {
			return errors.New("refuse Keycloak client-secret rotation: provider secret and protected binding state have drifted")
		}
		current, rotated, err := admin.rotateClientSecret(ctx, d.realm, clientUUID)
		if err != nil {
			return err
		}
		if rotated != oldLocal {
			return errors.New("Keycloak did not preserve the previously active client secret as the overlap credential")
		}
		data, err := json.Marshal(preparedKeycloakClientSecretRotation{Current: current, Rotated: rotated})
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

	material, err := loadPreparedKeycloakClientSecretRotation(prepared, key)
	if err != nil {
		return err
	}

	if phase == credential.RotationPrepared {
		if err := d.applyClientSecretBinding(ctx, material.Current); err != nil {
			_ = d.applyClientSecretBinding(ctx, material.Rotated)
			return fmt.Errorf("reconcile Keycloak client-secret consumer: %w", err)
		}
		if err := journal.Save(key, credential.RotationReconciled); err != nil {
			_ = d.applyClientSecretBinding(ctx, material.Rotated)
			return err
		}
		phase = credential.RotationReconciled
	}

	if phase == credential.RotationReconciled {
		if err := waitForKeycloakClientSecretAuthentication(ctx, admin, d.realm, d.clientID, material.Current); err != nil {
			_ = d.applyClientSecretBinding(ctx, material.Rotated)
			_ = journal.Save(key, credential.RotationPrepared)
			return fmt.Errorf("verify new Keycloak client secret: %w", err)
		}
		if err := waitForKeycloakClientSecretAuthentication(ctx, admin, d.realm, d.clientID, material.Rotated); err != nil {
			return fmt.Errorf("verify Keycloak client-secret overlap before retirement: %w", err)
		}
		if err := journal.Save(key, credential.RotationVerified); err != nil {
			return err
		}
		phase = credential.RotationVerified
	}

	if phase == credential.RotationVerified {
		if err := admin.retireRotatedClientSecret(ctx, d.realm, clientUUID); err != nil {
			return err
		}
		if err := waitForKeycloakClientSecretRejection(ctx, admin, d.realm, d.clientID, material.Rotated); err != nil {
			return err
		}
		if err := waitForKeycloakClientSecretAuthentication(ctx, admin, d.realm, d.clientID, material.Current); err != nil {
			return fmt.Errorf("new Keycloak client secret failed after retirement: %w", err)
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

func (d *KeycloakDriver) applyClientSecretBinding(ctx context.Context, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return errors.New("Keycloak client secret is empty")
	}
	if err := writeKeycloakClientSecretFile(keycloakClientSecretPath(d.instance, d.realm), secret); err != nil {
		return err
	}
	client := d.instance.PublicHTTPClient
	if client == nil {
		return errors.New("Keycloak realization returned no public HTTP client")
	}
	endpointIssuer := strings.TrimRight(d.instance.EndpointBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	publicIssuer := strings.TrimRight(d.instance.PublicBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	workloadIssuer := strings.TrimRight(d.instance.WorkloadBaseURL, "/") + "/realms/" + url.PathEscape(d.realm)
	discovery, err := FetchDiscoveryAt(ctx, client, endpointIssuer, publicIssuer)
	if err != nil {
		return err
	}
	workloadDiscovery := rebaseIdentityDiscovery(discovery, publicIssuer, workloadIssuer)
	if err := application.MaterializeIdentityBindingWithWorkloadDiscoveryMaterial(
		d.app,
		d.appFiles,
		string(capability.ProviderKeycloak),
		discovery,
		workloadDiscovery,
		d.clientID,
		secret,
		d.instance.TrustBundle,
	); err != nil {
		return err
	}
	d.clientSecret = secret
	d.discovery = discovery
	return nil
}

func (a *keycloakAdmin) resolveClientUUID(ctx context.Context, realm, clientID string) (string, error) {
	query := url.Values{}
	query.Set("clientId", clientID)
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/clients?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("resolve Keycloak client for secret rotation: HTTP %d: %s", status, body)
	}
	var clients []keycloakClient
	if err := json.Unmarshal([]byte(body), &clients); err != nil {
		return "", err
	}
	if len(clients) != 1 || strings.TrimSpace(clients[0].ID) == "" {
		return "", fmt.Errorf("resolve Keycloak client %q for secret rotation", clientID)
	}
	return clients[0].ID, nil
}

func loadPreparedKeycloakClientSecretRotation(store credential.PreparedMaterialStore, key string) (preparedKeycloakClientSecretRotation, error) {
	data, err := store.Load(key)
	if err != nil {
		return preparedKeycloakClientSecretRotation{}, err
	}
	if len(data) == 0 {
		return preparedKeycloakClientSecretRotation{}, errors.New("prepared Keycloak client-secret rotation state is missing")
	}
	var material preparedKeycloakClientSecretRotation
	if err := json.Unmarshal(data, &material); err != nil {
		return preparedKeycloakClientSecretRotation{}, err
	}
	material.Current = strings.TrimSpace(material.Current)
	material.Rotated = strings.TrimSpace(material.Rotated)
	if material.Current == "" || material.Rotated == "" || material.Current == material.Rotated {
		return preparedKeycloakClientSecretRotation{}, errors.New("prepared Keycloak client-secret rotation state is invalid")
	}
	return material, nil
}

func readKeycloakClientSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("stored Keycloak client secret is invalid")
	}
	return value, nil
}

func writeKeycloakClientSecretFile(path, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" || strings.ContainsAny(secret, "\r\n") {
		return errors.New("Keycloak client secret is invalid")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}


const keycloakClientSecretPropagationTimeout = 30 * time.Second

func waitForKeycloakClientSecretAuthentication(ctx context.Context, admin *keycloakAdmin, realm, clientID, secret string) error {
	waitCtx, cancel := context.WithTimeout(ctx, keycloakClientSecretPropagationTimeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var last error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(waitCtx, 5*time.Second)
		err := admin.verifyClientSecretAuthentication(attemptCtx, realm, clientID, secret)
		attemptCancel()
		if err == nil {
			return nil
		}
		last = err
		select {
		case <-waitCtx.Done():
			if last == nil {
				last = waitCtx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func waitForKeycloakClientSecretRejection(ctx context.Context, admin *keycloakAdmin, realm, clientID, secret string) error {
	waitCtx, cancel := context.WithTimeout(ctx, keycloakClientSecretPropagationTimeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	const requiredConsecutiveRejections = 6
	consecutive := 0
	var last error = errors.New("retired Keycloak client secret is still accepted")
	for {
		attemptCtx, attemptCancel := context.WithTimeout(waitCtx, 5*time.Second)
		err := admin.verifyClientSecretAuthentication(attemptCtx, realm, clientID, secret)
		attemptCancel()
		switch {
		case errors.Is(err, errKeycloakClientSecretRejected):
			consecutive++
			if consecutive >= requiredConsecutiveRejections {
				return nil
			}
			last = err
		case err == nil:
			consecutive = 0
			last = errors.New("retired Keycloak client secret is still accepted")
		default:
			consecutive = 0
			last = err
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("retired Keycloak client secret did not converge to rejection: %w", last)
		case <-ticker.C:
		}
	}
}
