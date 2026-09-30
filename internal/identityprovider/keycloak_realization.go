package identityprovider

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// KeycloakInstance is the runtime-neutral control surface consumed by the
// identity Core. Runtime-native deployment identity remains inside the
// realization that produced this instance.
type KeycloakInstance struct {
	StateDir         string
	EndpointBaseURL  string
	PublicBaseURL    string
	WorkloadBaseURL  string
	TrustBundle      []byte
	PublicHTTPClient *http.Client
	AdminHTTPClient  *http.Client
	AdminUsername    string
	AdminPassword    string
}

// KeycloakRealization is the deployment/runtime boundary for the managed
// Keycloak reference provider. Adding Kubernetes/OpenShift/cloud realizations
// must not require changes to Keycloak identity semantics.
type KeycloakRealization interface {
	Apply(context.Context) (KeycloakInstance, error)
	Existing(context.Context) (KeycloakInstance, error)
	Destroy(context.Context) error
}

type localKeycloakRealization struct {
	runtime   KeycloakRuntime
	lifecycle KeycloakLifecycle
	app       application.Manifest
	issuer    serviceaccess.Issuer
	dataDir   string
	namespace string
	files     KeycloakFiles
}

func newLocalKeycloakRealization(runtime KeycloakRuntime, app application.Manifest, issuer serviceaccess.Issuer, dataDir, namespace string) KeycloakRealization {
	return &localKeycloakRealization{
		runtime: runtime, lifecycle: NewKeycloakLifecycle(runtime), app: app,
		issuer: issuer, dataDir: dataDir, namespace: namespace,
	}
}

func (r *localKeycloakRealization) Apply(ctx context.Context) (KeycloakInstance, error) {
	files, err := EnsureKeycloakFilesAt(ctx, r.app, r.issuer, r.dataDir, r.namespace)
	if err != nil {
		return KeycloakInstance{}, err
	}
	r.files = files

	publicBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	if err := SetKeycloakCanonicalURL(files, publicBase); err != nil {
		return KeycloakInstance{}, err
	}
	if cleaner, ok := r.runtime.(legacyServiceCleaner); ok {
		if err := cleaner.RemoveProjectServices(ctx, files.Project, "keycloak-public", "keycloak-admin"); err != nil {
			return KeycloakInstance{}, err
		}
	}
	if err := r.lifecycle.Validate(ctx, files); err != nil {
		return KeycloakInstance{}, err
	}
	if err := r.lifecycle.Apply(ctx, files); err != nil {
		return KeycloakInstance{}, err
	}
	return r.instance(ctx, files, publicBase)
}

func (r *localKeycloakRealization) Existing(ctx context.Context) (KeycloakInstance, error) {
	files, err := ExistingKeycloakFilesAt(r.app, r.dataDir, r.namespace)
	if err != nil {
		return KeycloakInstance{}, err
	}
	r.files = files
	publicBase, err := localKeycloakPublicBaseURL(r.app, r.namespace, r.runtime, files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	return r.instance(ctx, files, publicBase)
}

func (r *localKeycloakRealization) Destroy(ctx context.Context) error {
	files := r.files
	if files.Dir == "" {
		var err error
		files, err = ExistingKeycloakFilesAt(r.app, r.dataDir, r.namespace)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	placement, err := application.ResolveProviderPlacement(r.app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	if placement.Scope != capability.ScopeApplication {
		return nil
	}
	if err := r.lifecycle.Destroy(ctx, files); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func (r *localKeycloakRealization) instance(ctx context.Context, files KeycloakFiles, publicBase string) (KeycloakInstance, error) {
	publicClient, err := keycloakPublicHTTPClient(files)
	if err != nil {
		return KeycloakInstance{}, err
	}
	adminClient, err := serviceaccess.NewHTTPClient(files.AdminAccess.Material, false)
	if err != nil {
		return KeycloakInstance{}, err
	}
	values, err := readProtectedEnv(files.Env)
	if err != nil {
		return KeycloakInstance{}, err
	}

	workloadBase := files.PublicURL
	trustBundle := append([]byte(nil), files.PublicAccess.Material.CA...)
	if isDevelopmentIdentityEnvironment(r.app.Environment) {
		if err := ensureLocalKeycloakDevelopmentRoute(ctx, r.app, r.namespace, r.runtime, r.issuer, files); err != nil {
			return KeycloakInstance{}, err
		}
		gatewayFiles, err := devgateway.FilesFor(localKeycloakTargetName(r.namespace))
		if err != nil {
			return KeycloakInstance{}, err
		}
		workloadBase = publicBase
		trustBundle, err = os.ReadFile(gatewayFiles.CA)
		if err != nil {
			return KeycloakInstance{}, err
		}
	}

	return KeycloakInstance{
		StateDir:         files.Dir,
		EndpointBaseURL:  files.PublicURL,
		PublicBaseURL:    publicBase,
		WorkloadBaseURL:  workloadBase,
		TrustBundle:      trustBundle,
		PublicHTTPClient: publicClient,
		AdminHTTPClient:  adminClient,
		AdminUsername:    values["BASEHARBOR_KEYCLOAK_ADMIN_USER"],
		AdminPassword:    values["BASEHARBOR_KEYCLOAK_ADMIN_PASSWORD"],
	}, nil
}

func localKeycloakTargetName(namespace string) string {
	if value := strings.TrimSpace(namespace); value != "" {
		return value
	}
	return "local"
}

func localKeycloakPublicBaseURL(app application.Manifest, namespace string, runtime KeycloakRuntime, files KeycloakFiles) (string, error) {
	if !isDevelopmentIdentityEnvironment(app.Environment) {
		return files.PublicURL, nil
	}
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return "", err
	}
	target := localKeycloakTargetName(namespace)
	var host string
	switch placement.Scope {
	case capability.ScopeShared:
		host, err = devaccess.SharedHost(target, "identity")
	case capability.ScopeApplication:
		host, err = devaccess.ApplicationHost(target, app.Name, "identity")
	case capability.ScopeExternal:
		return "", errors.New("external OIDC has no managed Keycloak public URL")
	}
	if err != nil {
		return "", err
	}
	return devgateway.URLForRuntime(target, host, runtime), nil
}

func ensureLocalKeycloakDevelopmentRoute(ctx context.Context, app application.Manifest, namespace string, runtime KeycloakRuntime, issuer serviceaccess.Issuer, files KeycloakFiles) error {
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return err
	}
	target := localKeycloakTargetName(namespace)
	var owner, key, host string
	switch placement.Scope {
	case capability.ScopeShared:
		owner = "shared/keycloak"
		key = owner + "/login"
		host, err = devaccess.SharedHost(target, "identity")
	case capability.ScopeApplication:
		owner = "app/" + app.Name + "/" + app.Environment
		key = owner + "/identity"
		host, err = devaccess.ApplicationHost(target, app.Name, "identity")
	case capability.ScopeExternal:
		return errors.New("external OIDC has no managed Keycloak development route")
	}
	if err != nil {
		return err
	}
	route := devgateway.Route{
		Key:        key,
		Host:       host,
		Upstream:   "https://" + devaccess.ProviderAlias(files.Project, "identity") + ":8443",
		Network:    files.ConsumerNetwork,
		TrustFile:  files.PublicAccess.Material.CA,
		ServerName: files.PublicAccess.Material.ServerName,
	}
	return devgateway.UpsertOwnerRoutes(ctx, runtime, issuer, target, owner, []devgateway.Route{route})
}

func keycloakClientSecretPath(instance KeycloakInstance, realm string) string {
	return filepath.Join(instance.StateDir, "scopes", realm, "client-secret")
}
