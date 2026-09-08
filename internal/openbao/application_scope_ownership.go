package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func CheckApplicationScopeOwnership(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
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
	managerCredentials, err := LoadAdminCredentials(files)
	if err != nil {
		return err
	}
	managerToken, err := loginManager(ctx, executor, files, managerCredentials)
	if err != nil {
		return err
	}

	roleName := applicationRoleName(identity)
	out, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao read -format=json auth/approle/role/%s`, roleName))
	if err != nil {
		return errors.New("OpenBao application AppRole is missing or unreadable")
	}
	var roleReply struct {
		Data struct {
			TokenPolicies        []string `json:"token_policies"`
			TokenNoDefaultPolicy bool     `json:"token_no_default_policy"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &roleReply); err != nil {
		return errors.New("OpenBao application AppRole returned an invalid response")
	}
	expectedPolicy := applicationPolicyName(identity)
	if !roleReply.Data.TokenNoDefaultPolicy || len(roleReply.Data.TokenPolicies) != 1 || roleReply.Data.TokenPolicies[0] != expectedPolicy {
		return errors.New("OpenBao application AppRole ownership does not match the expected BaseHarbor policy")
	}

	out, err = execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao policy read %s`, expectedPolicy))
	if err != nil {
		return errors.New("OpenBao application policy is missing or unreadable")
	}
	if normalizePolicy(out) != normalizePolicy(applicationPolicy(identity)) {
		return errors.New("OpenBao application policy differs from the BaseHarbor-managed definition; refusing destructive mutation")
	}
	return nil
}

func normalizePolicy(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}
