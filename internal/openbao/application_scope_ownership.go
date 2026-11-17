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
	_, _, err := inspectApplicationScopeOwnership(ctx, executor, files, identity, false)
	return err
}

// CheckApplicationScopeOwnershipForDestroy verifies every scope object that
// still exists, while treating an already-removed AppRole or policy as an
// idempotent destroy state.
func CheckApplicationScopeOwnershipForDestroy(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity) error {
	_, _, err := inspectApplicationScopeOwnership(ctx, executor, files, identity, true)
	return err
}

func inspectApplicationScopeOwnership(ctx context.Context, executor Executor, files bhruntime.Files, identity ApplicationIdentity, allowMissing bool) (bool, bool, error) {
	if err := validateApplicationIdentity(identity); err != nil {
		return false, false, err
	}
	state, err := Inspect(ctx, executor, files)
	if err != nil {
		return false, false, err
	}
	if !state.Initialized {
		return false, false, ErrNotInitialized
	}
	if state.Sealed {
		return false, false, ErrSealed
	}
	managerCredentials, err := LoadAdminCredentials(files)
	if err != nil {
		return false, false, err
	}
	managerToken, err := loginManager(ctx, executor, files, managerCredentials)
	if err != nil {
		return false, false, err
	}

	roleName := applicationRoleName(identity)
	out, err := execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao read -format=json auth/approle/role/%s`, roleName))
	rolePresent := err == nil
	if err != nil {
		if !allowMissing || !isMissingApplicationRole(err, roleName) {
			return false, false, fmt.Errorf("OpenBao application AppRole is missing or unreadable: %s", openBaoCommandFailureDetail(err))
		}
	} else {
		var roleReply struct {
			Data struct {
				TokenPolicies        []string `json:"token_policies"`
				TokenNoDefaultPolicy bool     `json:"token_no_default_policy"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &roleReply); err != nil {
			return false, false, errors.New("OpenBao application AppRole returned an invalid response")
		}
		expectedPolicy := applicationPolicyName(identity)
		if !roleReply.Data.TokenNoDefaultPolicy || len(roleReply.Data.TokenPolicies) != 1 || roleReply.Data.TokenPolicies[0] != expectedPolicy {
			return false, false, errors.New("OpenBao application AppRole ownership does not match the expected BaseHarbor policy")
		}
	}
	expectedPolicy := applicationPolicyName(identity)
	out, err = execWithToken(ctx, executor, files, managerToken, fmt.Sprintf(`exec bao policy read %s`, expectedPolicy))
	policyPresent := err == nil
	if err != nil {
		if !allowMissing || !isMissingApplicationPolicy(err, expectedPolicy) {
			return false, false, fmt.Errorf("OpenBao application policy is missing or unreadable: %s", openBaoCommandFailureDetail(err))
		}
	} else if normalizePolicy(out) != normalizePolicy(applicationPolicy(identity)) {
		return false, false, errors.New("OpenBao application policy differs from the BaseHarbor-managed definition; refusing destructive mutation")
	}
	return rolePresent, policyPresent, nil
}

func isMissingApplicationRole(err error, role string) bool {
	detail := strings.ToLower(openBaoCommandFailureDetail(err))
	return strings.Contains(detail, "no value found at auth/approle/role/"+strings.ToLower(role))
}

func isMissingApplicationPolicy(err error, policy string) bool {
	detail := strings.ToLower(openBaoCommandFailureDetail(err))
	return strings.Contains(detail, "no policy named: "+strings.ToLower(policy)) || strings.Contains(detail, "no policy named "+strings.ToLower(policy))
}

func openBaoCommandFailureDetail(err error) string {
	message := strings.TrimSpace(err.Error())
	if strings.HasPrefix(message, "compose ") {
		if _, detail, ok := strings.Cut(message, ": "); ok {
			message = strings.TrimSpace(detail)
		}
	}
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || strings.Contains(message, "BAO_TOKEN") {
		return "command failed"
	}
	if len(message) > 160 {
		message = message[:157] + "..."
	}
	return message
}

func normalizePolicy(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, "\n")
}
