package openbao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func CheckApplicationProvisioning(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
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
	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return err
	}

	policyName := applicationPolicyName(identity)
	roleName := applicationRoleName(identity)
	checks := []struct {
		path string
		need []string
	}{
		{path: "sys/policies/acl/" + policyName, need: []string{"create", "update", "read", "delete"}},
		{path: "auth/approle/role/" + roleName, need: []string{"create", "update", "read", "delete"}},
		{path: "auth/approle/role/" + roleName + "/role-id", need: []string{"read"}},
		{path: "auth/approle/role/" + roleName + "/secret-id", need: []string{"create", "update"}},
	}
	for _, check := range checks {
		out, err := execWithToken(ctx, executor, files, token, fmt.Sprintf(`exec bao token capabilities %s`, check.path))
		if err != nil {
			return fmt.Errorf("inspect OpenBao manager capabilities for %s: %w", check.path, err)
		}
		capabilities := parseCapabilities(out)
		for _, required := range check.need {
			if !capabilities[required] {
				return errors.New("OpenBao manager lacks application identity provisioning permissions; bootstrap or reconcile the trust plane with the current BaseHarbor version")
			}
		}
	}
	return nil
}

func parseCapabilities(value string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	}) {
		item = strings.TrimSpace(item)
		if item != "" {
			result[item] = true
		}
	}
	return result
}
