package openbao

import (
	"context"
	"errors"
	"fmt"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func InspectApplicationScope(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, credentialsPath string) error {
	if err := validateApplicationIdentity(identity); err != nil {
		return err
	}
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return err
	}
	if !state.Initialized {
		return ErrNotInitialized
	}
	if state.Sealed {
		return ErrSealed
	}
	credentials, err := loadApplicationCredentials(credentialsPath)
	if err != nil {
		return err
	}
	token, err := loginApplication(ctx, executor, files, credentials)
	if err != nil {
		return err
	}

	path := applicationSecretPath(identity)
	checks := []struct {
		path string
		need []string
	}{
		{path: "baseharbor/data/" + path, need: []string{"create", "update", "read", "delete"}},
		{path: "baseharbor/metadata/" + path, need: []string{"read", "delete"}},
	}
	for _, check := range checks {
		out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao token capabilities %s`, check.path))
		if err != nil {
			return fmt.Errorf("inspect OpenBao application capabilities: %w", err)
		}
		capabilities := parseCapabilities(out)
		for _, required := range check.need {
			if !capabilities[required] {
				return errors.New("OpenBao application identity is missing required secret-scope capabilities")
			}
		}
	}
	return CheckApplicationScopeOwnership(ctx, executor, files, identity)
}
