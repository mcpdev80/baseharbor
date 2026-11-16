package identityprovider

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	keycloakKeyProviderType = "org.keycloak.keys.KeyProvider"
	keycloakRSAProviderID   = "rsa-generated"
	managedSigningPrefix    = "baseharbor-signing-"
)

type keycloakComponent struct {
	ID           string              `json:"id,omitempty"`
	Name         string              `json:"name"`
	ProviderID   string              `json:"providerId"`
	ProviderType string              `json:"providerType"`
	ParentID     string              `json:"parentId"`
	Config       map[string][]string `json:"config"`
}

type keycloakKeyMetadata struct {
	ProviderID       string `json:"providerId"`
	ProviderPriority int    `json:"providerPriority"`
	Kid              string `json:"kid"`
	Status           string `json:"status,omitempty"`
	Type             string `json:"type,omitempty"`
	Algorithm        string `json:"algorithm,omitempty"`
	PublicKey        string `json:"publicKey,omitempty"`
	Certificate      string `json:"certificate,omitempty"`
}

type keycloakKeysMetadata struct {
	Active map[string]string     `json:"active"`
	Keys   []keycloakKeyMetadata `json:"keys"`
}

type keycloakJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

type keycloakJWKS struct {
	Keys []keycloakJWK `json:"keys"`
}

func (a *keycloakAdmin) realmRepresentation(ctx context.Context, realm string) (keycloakRealm, error) {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm), nil)
	if err != nil {
		return keycloakRealm{}, err
	}
	if status != http.StatusOK {
		return keycloakRealm{}, fmt.Errorf("read Keycloak realm for signing-key management: HTTP %d: %s", status, body)
	}
	var representation keycloakRealm
	if err := json.Unmarshal([]byte(body), &representation); err != nil {
		return keycloakRealm{}, fmt.Errorf("decode Keycloak realm for signing-key management: %w", err)
	}
	if strings.TrimSpace(representation.ID) == "" {
		// Keycloak commonly uses the realm name as realm id. Keeping this fallback
		// also makes the operation robust across older realm representations.
		representation.ID = representation.Realm
	}
	if strings.TrimSpace(representation.ID) == "" {
		return keycloakRealm{}, errors.New("Keycloak realm identity is empty")
	}
	return representation, nil
}

func (a *keycloakAdmin) keyProviderComponents(ctx context.Context, realm string) ([]keycloakComponent, error) {
	representation, err := a.realmRepresentation(ctx, realm)
	if err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("parent", representation.ID)
	query.Set("type", keycloakKeyProviderType)
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/components?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list Keycloak signing providers: HTTP %d: %s", status, body)
	}
	var components []keycloakComponent
	if err := json.Unmarshal([]byte(body), &components); err != nil {
		return nil, fmt.Errorf("decode Keycloak signing providers: %w", err)
	}
	return components, nil
}

func (a *keycloakAdmin) createManagedSigningProvider(ctx context.Context, realm, name string, priority int) (keycloakComponent, error) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, managedSigningPrefix) {
		return keycloakComponent{}, fmt.Errorf("managed Keycloak signing provider %q must use %q prefix", name, managedSigningPrefix)
	}
	representation, err := a.realmRepresentation(ctx, realm)
	if err != nil {
		return keycloakComponent{}, err
	}
	component := keycloakComponent{
		Name:         name,
		ProviderID:   keycloakRSAProviderID,
		ProviderType: keycloakKeyProviderType,
		ParentID:     representation.ID,
		Config: map[string][]string{
			"priority":  {strconv.Itoa(priority)},
			"enabled":   {"true"},
			"active":    {"true"},
			"keySize":   {"2048"},
			"algorithm": {"RS256"},
		},
	}
	status, body, err := a.do(ctx, http.MethodPost, "/admin/realms/"+url.PathEscape(realm)+"/components", component)
	if err != nil {
		return keycloakComponent{}, err
	}
	if status != http.StatusCreated {
		return keycloakComponent{}, fmt.Errorf("create Keycloak signing provider: HTTP %d: %s", status, body)
	}
	components, err := a.keyProviderComponents(ctx, realm)
	if err != nil {
		return keycloakComponent{}, err
	}
	for _, candidate := range components {
		if candidate.Name == name && candidate.ProviderID == keycloakRSAProviderID && strings.TrimSpace(candidate.ID) != "" {
			return candidate, nil
		}
	}
	return keycloakComponent{}, fmt.Errorf("created Keycloak signing provider %q cannot be resolved", name)
}

func (a *keycloakAdmin) updateSigningProviderPriority(ctx context.Context, realm string, component keycloakComponent, priority int) error {
	if !strings.HasPrefix(strings.TrimSpace(component.Name), managedSigningPrefix) {
		return fmt.Errorf("refuse to mutate non-BaseHarbor Keycloak signing provider %q", component.Name)
	}
	if strings.TrimSpace(component.ID) == "" {
		return errors.New("Keycloak signing provider id is empty")
	}
	if component.Config == nil {
		component.Config = map[string][]string{}
	}
	component.Config["priority"] = []string{strconv.Itoa(priority)}
	component.Config["enabled"] = []string{"true"}
	component.Config["active"] = []string{"true"}
	status, body, err := a.do(ctx, http.MethodPut, "/admin/realms/"+url.PathEscape(realm)+"/components/"+url.PathEscape(component.ID), component)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("promote Keycloak signing provider: HTTP %d: %s", status, body)
	}
	return nil
}

func (a *keycloakAdmin) deleteManagedSigningProvider(ctx context.Context, realm string, component keycloakComponent) error {
	if !strings.HasPrefix(strings.TrimSpace(component.Name), managedSigningPrefix) {
		return fmt.Errorf("refuse to delete non-BaseHarbor Keycloak signing provider %q", component.Name)
	}
	if strings.TrimSpace(component.ID) == "" {
		return errors.New("Keycloak signing provider id is empty")
	}
	status, body, err := a.do(ctx, http.MethodDelete, "/admin/realms/"+url.PathEscape(realm)+"/components/"+url.PathEscape(component.ID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("delete Keycloak signing provider: HTTP %d: %s", status, body)
	}
	return nil
}

func (a *keycloakAdmin) signingKeys(ctx context.Context, realm string) (keycloakKeysMetadata, error) {
	status, body, err := a.do(ctx, http.MethodGet, "/admin/realms/"+url.PathEscape(realm)+"/keys", nil)
	if err != nil {
		return keycloakKeysMetadata{}, err
	}
	if status != http.StatusOK {
		return keycloakKeysMetadata{}, fmt.Errorf("read Keycloak signing keys: HTTP %d: %s", status, body)
	}
	var metadata keycloakKeysMetadata
	if err := json.Unmarshal([]byte(body), &metadata); err != nil {
		return keycloakKeysMetadata{}, fmt.Errorf("decode Keycloak signing keys: %w", err)
	}
	return metadata, nil
}

func managedSigningComponents(components []keycloakComponent) []keycloakComponent {
	out := make([]keycloakComponent, 0, len(components))
	for _, component := range components {
		if component.ProviderID == keycloakRSAProviderID && strings.HasPrefix(component.Name, managedSigningPrefix) {
			out = append(out, component)
		}
	}
	return out
}

func componentPriority(component keycloakComponent) int {
	values := component.Config["priority"]
	if len(values) == 0 {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(values[0]))
	return value
}

func (a *keycloakAdmin) ensureManagedSigningProvider(ctx context.Context, realm string) (keycloakComponent, string, error) {
	components, err := a.keyProviderComponents(ctx, realm)
	if err != nil {
		return keycloakComponent{}, "", err
	}
	managed := managedSigningComponents(components)
	if len(managed) == 0 {
		component, err := a.createManagedSigningProvider(ctx, realm, managedSigningPrefix+"initial", 1000)
		if err != nil {
			return keycloakComponent{}, "", err
		}
		managed = []keycloakComponent{component}
	}
	best := managed[0]
	for _, candidate := range managed[1:] {
		if componentPriority(candidate) > componentPriority(best) {
			best = candidate
		}
	}
	keys, err := a.signingKeys(ctx, realm)
	if err != nil {
		return keycloakComponent{}, "", err
	}
	active := strings.TrimSpace(keys.Active["RS256"])
	managedKid := kidForSigningComponent(keys, best.ID)
	if managedKid == "" {
		return keycloakComponent{}, "", fmt.Errorf("BaseHarbor signing provider %q has no published RS256 key", best.Name)
	}
	if active != managedKid {
		if err := a.updateSigningProviderPriority(ctx, realm, best, 10000); err != nil {
			return keycloakComponent{}, "", err
		}
		best.Config["priority"] = []string{"10000"}
		keys, err = a.signingKeys(ctx, realm)
		if err != nil {
			return keycloakComponent{}, "", err
		}
		active = strings.TrimSpace(keys.Active["RS256"])
		if active != managedKid {
			return keycloakComponent{}, "", fmt.Errorf("BaseHarbor signing provider %q did not become active", best.Name)
		}
	}
	return best, active, nil
}

func (a *keycloakAdmin) ensureSigningRotationProbeClient(ctx context.Context, realm, clientID, secret string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	secret = strings.TrimSpace(secret)
	if clientID == "" || secret == "" {
		return "", errors.New("Keycloak signing rotation probe client is incomplete")
	}
	return a.reconcileClient(ctx, realm, keycloakClient{
		ClientID:                  clientID,
		Name:                      "BaseHarbor signing rotation probe",
		Enabled:                   true,
		Protocol:                  "openid-connect",
		PublicClient:              false,
		StandardFlowEnabled:       false,
		DirectAccessGrantsEnabled: false,
		ServiceAccountsEnabled:    true,
		Secret:                    secret,
		Attributes: map[string]string{
			"baseharbor.owner": "signing-key-rotation",
		},
	})
}

func (a *keycloakAdmin) mintSigningProbeToken(ctx context.Context, realm, clientID, secret string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", secret)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(a.endpoint, "/")+"/realms/"+url.PathEscape(realm)+"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mint Keycloak signing rotation probe token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mint Keycloak signing rotation probe token: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	payload.AccessToken = strings.TrimSpace(payload.AccessToken)
	if payload.AccessToken == "" {
		return "", errors.New("Keycloak signing rotation probe token is empty")
	}
	return payload.AccessToken, nil
}

func (a *keycloakAdmin) deleteSigningRotationProbeClient(ctx context.Context, realm, clientUUID string) error {
	clientUUID = strings.TrimSpace(clientUUID)
	if clientUUID == "" {
		return nil
	}
	status, body, err := a.do(ctx, http.MethodDelete, "/admin/realms/"+url.PathEscape(realm)+"/clients/"+url.PathEscape(clientUUID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("delete Keycloak signing rotation probe client: HTTP %d: %s", status, body)
	}
	return nil
}

func kidForSigningComponent(keys keycloakKeysMetadata, componentID string) string {
	componentID = strings.TrimSpace(componentID)
	for _, key := range keys.Keys {
		if strings.TrimSpace(key.ProviderID) == componentID && strings.EqualFold(strings.TrimSpace(key.Algorithm), "RS256") {
			return strings.TrimSpace(key.Kid)
		}
	}
	return ""
}

func fetchKeycloakJWKS(ctx context.Context, client *http.Client, baseURL, realm string) (keycloakJWKS, error) {
	if client == nil {
		return keycloakJWKS{}, errors.New("Keycloak HTTP client is required")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/realms/" + url.PathEscape(realm) + "/protocol/openid-connect/certs"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return keycloakJWKS{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return keycloakJWKS{}, fmt.Errorf("read Keycloak JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return keycloakJWKS{}, fmt.Errorf("read Keycloak JWKS: HTTP %d", resp.StatusCode)
	}
	var jwks keycloakJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return keycloakJWKS{}, fmt.Errorf("decode Keycloak JWKS: %w", err)
	}
	return jwks, nil
}

func jwksContainsKid(jwks keycloakJWKS, kid string) bool {
	kid = strings.TrimSpace(kid)
	for _, key := range jwks.Keys {
		if key.Kid == kid {
			return true
		}
	}
	return false
}

func jwtKid(token string) (string, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return "", errors.New("JWT must have three segments")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode JWT header: %w", err)
	}
	var value struct {
		Algorithm string `json:"alg"`
		Kid       string `json:"kid"`
	}
	if err := json.Unmarshal(header, &value); err != nil {
		return "", fmt.Errorf("decode JWT header JSON: %w", err)
	}
	if value.Algorithm != "RS256" || strings.TrimSpace(value.Kid) == "" {
		return "", fmt.Errorf("JWT signing metadata is unsupported")
	}
	return strings.TrimSpace(value.Kid), nil
}

func verifyRS256JWTWithJWKS(token string, jwks keycloakJWKS) error {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return errors.New("JWT must have three segments")
	}
	kid, err := jwtKid(token)
	if err != nil {
		return err
	}
	var selected *keycloakJWK
	for i := range jwks.Keys {
		if jwks.Keys[i].Kid == kid && jwks.Keys[i].Kty == "RSA" {
			selected = &jwks.Keys[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("JWKS does not contain JWT kid %q", kid)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(selected.N)
	if err != nil || len(nBytes) == 0 {
		return fmt.Errorf("decode JWKS RSA modulus for kid %q", kid)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(selected.E)
	if err != nil || len(eBytes) == 0 {
		return fmt.Errorf("decode JWKS RSA exponent for kid %q", kid)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e <= 1 {
		return fmt.Errorf("invalid JWKS RSA exponent for kid %q", kid)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode JWT signature: %w", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	publicKey := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, sum[:], signature); err != nil {
		return fmt.Errorf("verify JWT kid %q with current JWKS: %w", kid, err)
	}
	return nil
}
