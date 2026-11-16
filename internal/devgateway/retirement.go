package devgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// The existing route owner is retained until runtime retirement is verified.
// Gateway TLS/configuration remain at their current paths for re-add/recovery.
func retireLastGatewayRoutes(ctx context.Context, runtime Runtime, files Files, previous state) error {
	provider, ok := runtime.(bhruntime.RuntimeRetirementProvider)
	if !ok {
		return errors.New("gateway retirement requires native ownership inspection and data-preserving lifecycle support")
	}
	info, err := os.Lstat(files.Compose)
	if errors.Is(err, os.ErrNotExist) {
		return verifyEmptyGatewayInactive(ctx, runtime, files)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("gateway runtime definition is unprotected or noncanonical")
	}
	actual, err := os.ReadFile(files.Compose)
	if err != nil || !bytes.Equal(actual, []byte(renderCompose(files, previous.Routes, nil, previous.HostPort))) {
		return errors.New("gateway runtime definition differs from its authoritative routes")
	}
	info, err = os.Lstat(files.Env)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("gateway runtime environment is unprotected or noncanonical")
	}
	_, err = bhruntime.RetireOwnedRuntime(ctx, provider, bhruntime.Files{Project: files.Project, Compose: files.Compose, Env: files.Env}, []bhruntime.ProjectResource{{Kind: "container", Name: files.Project + "-dev-gateway-1"}})
	return err
}

func verifyEmptyGatewayInactive(ctx context.Context, runtime Runtime, files Files) error {
	provider, ok := runtime.(bhruntime.RuntimeRetirementProvider)
	if !ok {
		return errors.New("gateway inactivity cannot be verified by this runtime")
	}
	resources, err := provider.InspectProjectResources(ctx, files.Project, []bhruntime.ProjectResource{{Kind: "container", Name: files.Project + "-dev-gateway-1"}})
	if err != nil {
		return err
	}
	if len(resources) != 0 {
		return fmt.Errorf("gateway runtime remains without active routes; retain owner state for explicit recovery")
	}
	return nil
}
