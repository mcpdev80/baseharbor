package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/stableid"
)

// EnsureCoreIdentity realizes installation Identity through the existing native
// provider. No Application, repository, workload binding or login is required.
// Application placement preferences do not redefine this installation scope.
func EnsureCoreIdentity(ctx context.Context, runtime KeycloakRuntime, issuer serviceaccess.Issuer, dataDir, namespace, installationID string) (string, error) {
	if err := stableid.ValidateUUIDv4("installation", installationID); err != nil {
		return "", err
	}
	spec := application.Manifest{Version: application.CurrentVersion, Name: "core", Environment: "prod", Services: application.Services{Identity: true}}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
	files, err := ensureKeycloakFilesForPlacement(ctx, spec, issuer, dataDir, namespace, placement)
	if err != nil {
		return "", err
	}
	if err = SetKeycloakCanonicalURL(files, files.PublicURL); err != nil {
		return "", err
	}
	if err = runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return "", err
	}
	if err = NewKeycloakLifecycle(runtime).Apply(ctx, files); err != nil {
		return "", err
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return "", err
	}
	ownership := map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "installation", "baseharbor.installation": installationID}
	realm := keycloakRealm{Realm: "baseharbor", Enabled: true, DisplayName: "BaseHarbor", SSLRequired: "all", BruteForceProtected: true, RegistrationAllowed: false, Attributes: ownership}
	if err = admin.reconcileRealm(ctx, realm); err != nil {
		return "", errors.New("Core identity realm reconciliation failed; inspect protected provider diagnostics")
	}
	client, err := keycloakPublicHTTPClient(files)
	if err != nil {
		return "", err
	}
	endpoint := files.PublicURL + "/realms/" + url.PathEscape(realm.Realm)
	if _, err = FetchDiscoveryAt(ctx, client, endpoint, endpoint); err != nil {
		return "", err
	}
	return endpoint, nil
}

func VerifyCoreIdentity(ctx context.Context, dataDir, namespace, installationID, expectedIssuer string) error {
	if err := stableid.ValidateUUIDv4("installation", installationID); err != nil {
		return err
	}
	spec := application.Manifest{Version: application.CurrentVersion, Name: "core", Environment: "prod", Services: application.Services{Identity: true}}
	placement := capability.ProviderPlacement{Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor, SharingBoundary: "core"}
	dir, _, err := keycloakStateIdentity(spec, placement, dataDir, namespace)
	if err != nil {
		return err
	}
	// Read existing protected material only; probing readiness must not provision.
	files, err := existingCoreKeycloakFiles(dir)
	if err != nil {
		return err
	}
	if files.PublicURL+"/realms/baseharbor" != expectedIssuer {
		return errors.New("Core Identity destination differs from the owned installation")
	}
	admin, err := operatorKeycloakAdmin(ctx, files)
	if err != nil {
		return err
	}
	status, body, err := admin.do(ctx, "GET", "/admin/realms/baseharbor", nil)
	if err != nil {
		return err
	}
	var realm keycloakRealm
	if status != 200 || json.Unmarshal([]byte(body), &realm) != nil || !realm.Enabled || !keycloakRealmOwnedBy(realm, map[string]string{"baseharbor.owner": "baseharbor", "baseharbor.scope": "installation", "baseharbor.installation": installationID}) {
		return errors.New("Core Identity readiness or ownership verification failed")
	}
	client, err := keycloakPublicHTTPClient(files)
	if err != nil {
		return err
	}
	_, err = FetchDiscoveryAt(ctx, client, expectedIssuer, expectedIssuer)
	return err
}

func existingCoreKeycloakFiles(dir string) (KeycloakFiles, error) {
	values, err := readProtectedEnv(filepath.Join(dir, "runtime.env"))
	if err != nil {
		return KeycloakFiles{}, err
	}
	port, err := parseIdentityPort(values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"])
	if err != nil {
		return KeycloakFiles{}, err
	}
	policy, err := serviceaccess.Resolve("prod", "keycloak-public", serviceaccess.AuthenticationNative)
	if err != nil {
		return KeycloakFiles{}, err
	}
	policy.ServerName = keycloakPublicHost
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(dir, "native-tls", "pki"))
	if err != nil {
		return KeycloakFiles{}, err
	}
	return KeycloakFiles{Dir: dir, Env: filepath.Join(dir, "runtime.env"), PublicPort: port, AdminPort: port, PublicURL: "https://" + keycloakPublicHost + ":" + strconv.Itoa(port), AdminURL: "https://127.0.0.1:" + strconv.Itoa(port), PublicAccess: serviceaccess.HTTPGatewayFiles{Material: material}, AdminAccess: serviceaccess.HTTPGatewayFiles{Material: material}}, nil
}
