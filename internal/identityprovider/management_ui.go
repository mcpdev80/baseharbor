package identityprovider

import (
	"errors"
	"path/filepath"
	"strconv"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func ExistingKeycloakFilesAt(app application.Manifest, dataDir, namespace string) (KeycloakFiles, error) {
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return KeycloakFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return KeycloakFiles{}, errors.New("external OIDC has no BaseHarbor-owned Keycloak management surface")
	}
	dir, project, err := keycloakStateIdentity(app, placement, dataDir, namespace)
	if err != nil {
		return KeycloakFiles{}, err
	}
	envPath := filepath.Join(dir, "runtime.env")
	values, err := readProtectedEnv(envPath)
	if err != nil {
		return KeycloakFiles{}, err
	}
	publicPort, err := parseIdentityPort(values["BASEHARBOR_KEYCLOAK_PUBLIC_PORT"])
	if err != nil {
		return KeycloakFiles{}, err
	}
	consumer, err := application.IdentityProviderNetworkName(app, namespace)
	if err != nil {
		return KeycloakFiles{}, err
	}

	publicPolicy, err := serviceaccess.Resolve(app.Environment, "keycloak-public", serviceaccess.AuthenticationNative)
	if err != nil {
		return KeycloakFiles{}, err
	}
	publicPolicy.ServerName = keycloakPublicHost
	nativeMaterial, err := serviceaccess.ExistingTLSMaterial(publicPolicy, filepath.Join(dir, "native-tls", "pki"))
	if err != nil {
		return KeycloakFiles{}, err
	}
	nativeMaterial, err = projectKeycloakTLSMaterial(filepath.Join(dir, "native-tls", "runtime"), nativeMaterial)
	if err != nil {
		return KeycloakFiles{}, err
	}

	publicURL := "https://" + keycloakPublicHost + ":" + strconv.Itoa(publicPort)
	canonicalPublicURL := publicURL
	if devaccess.Enabled(app.Environment) {
		var host string
		if placement.Scope == capability.ScopeShared {
			host, err = devaccess.SharedHost(namespace, "identity")
		} else {
			host, err = devaccess.ApplicationHost(namespace, app.Name, "identity")
		}
		if err != nil {
			return KeycloakFiles{}, err
		}
		canonicalPublicURL = devgateway.URLForTarget(namespace, host)
	}
	return KeycloakFiles{
		Dir:                dir,
		Compose:            filepath.Join(dir, "compose.yaml"),
		Env:                envPath,
		Project:            project,
		ConsumerNetwork:    consumer,
		InternalNetwork:    consumer + "-internal",
		PublicPort:         publicPort,
		AdminPort:          publicPort,
		PublicURL:          publicURL,
		CanonicalPublicURL: canonicalPublicURL,
		AdminURL:           "https://127.0.0.1:" + strconv.Itoa(publicPort),
		PublicAccess: serviceaccess.HTTPGatewayFiles{
			Dir:      filepath.Join(dir, "native-tls"),
			Material: nativeMaterial,
		},
		AdminAccess: serviceaccess.HTTPGatewayFiles{
			Dir:      filepath.Join(dir, "native-tls"),
			Material: nativeMaterial,
		},
	}, nil
}

func ExistingKeycloakFiles(app application.Manifest) (KeycloakFiles, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return KeycloakFiles{}, err
	}
	return ExistingKeycloakFilesAt(app, dataDir, "")
}

func KeycloakManagementSurfaces(app application.Manifest, dataDir, namespace string) ([]application.ManagementUISurface, error) {
	if !app.Services.IdentityManagementUI {
		return nil, nil
	}
	placement, err := application.ResolveProviderPlacement(app, capability.ProviderKeycloak)
	if err != nil {
		return nil, err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil, nil
	}
	files, err := ExistingKeycloakFilesAt(app, dataDir, namespace)
	if err != nil {
		return nil, err
	}
	return []application.ManagementUISurface{
		{
			Service: "identity-login", Purpose: application.ProviderInterfaceUserFacing,
			URL: files.CanonicalPublicURL, Authentication: "oidc",
		},
		{
			Service: "identity-admin", Purpose: application.ProviderInterfaceAdministration,
			URL: files.AdminURL, Authentication: "keycloak-native",
		},
	}, nil
}
