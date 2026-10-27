package application

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

// Verify the same canonical discovery fields that materialization writes.
// Host and workload metadata may legitimately have distinct endpoint hosts.
func VerifyIdentityDiscoveryBinding(files RuntimeFiles, provider, clientID string, host, workload IdentityDiscovery) error {
	for root, discovery := range map[string]IdentityDiscovery{
		files.Bindings: host, workloadServiceBindingProjectionDir(files): workload,
	} {
		entries := identityBindingEntries(provider, discovery, clientID)
		entries["oidc.end-session-endpoint"] = discovery.EndSessionEndpoint
		for name, expected := range entries {
			path := filepath.Join(root, IdentityBindingName, name)
			if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) && expected == "" {
				continue
			}
			data, err := externalprovider.ReadMaterialReference((&url.URL{Scheme: "file", Path: path}).String(), false)
			if err != nil || strings.TrimSpace(string(data)) != expected {
				return errors.New("identity discovery delivery is unavailable or differs from verified metadata")
			}
		}
	}
	runtimeValues, err := readRuntimeEnv(files.Env)
	if err != nil {
		return errors.New("identity discovery runtime environment is unavailable")
	}
	appValues, err := loadApplicationEnvValues(files.ApplicationEnv)
	if err != nil || runtimeValues["IDENTITY_CONTAINER_ISSUER"] != workload.Issuer || appValues["OIDC_ISSUER"] != workload.Issuer {
		return errors.New("identity issuer environment differs from verified workload metadata")
	}
	return nil
}
