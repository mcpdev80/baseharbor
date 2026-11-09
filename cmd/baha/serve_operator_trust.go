package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/auth"
	"github.com/mcpdev80/baseharbor/internal/controlplaneruntime"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

// resolveServeOperatorOIDCTrustFile gives `baha serve` the same managed operator
// CA trust `baha login` already applies, so a managed Keycloak issuer verifies
// without OS system-trust surgery.
//
// Resolution keeps the documented precedence: an explicit
// BASEHARBOR_OPERATOR_OIDC_CA_FILE first (fail closed when it is unusable),
// then the managed CA the Core bootstrap wrote for the selected Target, then the
// state-wide default location. When no managed CA material exists, the empty
// result preserves the previous behavior exactly: discovery trusts only the
// system pool. The resolved file is always additive; TLS verification is never
// disabled and the system pool is never replaced.
func resolveServeOperatorOIDCTrustFile(ctx context.Context, cfg controlplaneruntime.Config) (string, error) {
	if explicit := strings.TrimSpace(os.Getenv(operatorauth.EnvTrustFile)); explicit != "" {
		data, err := os.ReadFile(explicit)
		if err != nil {
			return "", fmt.Errorf("read operator OIDC CA file %s: %w", explicit, err)
		}
		if !auth.PEMLooksLikeCertificate(data) {
			return "", fmt.Errorf("operator OIDC CA file %s is not a PEM certificate", explicit)
		}
		return explicit, nil
	}
	for _, candidate := range serveOperatorOIDCCandidates(ctx, cfg) {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		data, err := os.ReadFile(candidate)
		if err != nil || !auth.PEMLooksLikeCertificate(data) {
			continue
		}
		return candidate, nil
	}
	return "", nil
}

// serveOperatorOIDCCandidates lists the managed CA locations owned by the Core
// this server would address: the selected Target's state root first, then the
// state-wide default. A failed Target resolution is not an error: the state-wide
// default and the unchanged system-trust behavior remain available.
func serveOperatorOIDCCandidates(ctx context.Context, cfg controlplaneruntime.Config) []string {
	var candidates []string
	name := strings.TrimSpace(cfg.ConnectorAuthorityTarget)
	if name == "" {
		if target, err := effectiveTarget(ctx); err == nil {
			name = strings.TrimSpace(target.Name)
		}
	}
	if name != "" {
		if root, err := deployment.TargetStateRoot(name); err == nil && root != "" {
			candidates = append(candidates, filepath.Join(root, operatorauth.DefaultManagedCAFile))
		}
	}
	return append(candidates, operatorauth.DefaultTrustPath())
}
