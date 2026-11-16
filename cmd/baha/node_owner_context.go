package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// Never derive the tenant from CLI text, target defaults or a locally stored
// unverified JWT. The Core answers only after authenticating the OIDC bearer,
// resolving a unique membership and checking create permission.
func requestNodeOwnerTenant(ctx context.Context, coreTarget, environment, coreURL, caFile string) (string, error) {
	if err := validateCoreURL(coreURL); err != nil {
		return "", err
	}
	session, err := operatorauth.LoadSession(coreTarget, environment)
	if err != nil || !session.ValidAt(time.Now()) {
		return "", machine.NewError(machine.ErrorAuthenticationFailed, "node enrollment requires a verified operator session", "Run the Core's first-owner onboarding or baha login, then retry.", false)
	}
	client, err := nodeHTTPSClient(caFile)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(coreURL, "/")+targetenrollment.OwnerContextPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+session.IDToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", machine.Wrap(machine.ErrorProviderUnavailable, err, "Verify the Core API endpoint and pinned CA.", true)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(body) > 4096 {
		return "", fmt.Errorf("Core returned an invalid tenant ownership response")
	}
	if resp.StatusCode != http.StatusOK {
		return "", machine.NewError(machine.ErrorAuthenticationFailed,
			fmt.Sprintf("Core refused verified operator tenant ownership (HTTP %d)", resp.StatusCode),
			"Complete the Core first-owner OIDC setup and ensure exactly one authorized tenant membership.", false)
	}
	var owner struct {
		ContractVersion string `json:"contract_version"`
		TenantID        string `json:"tenant_id"`
	}
	if err := json.Unmarshal(body, &owner); err != nil || owner.ContractVersion != machine.ContractVersion || !canonicalTenantID(owner.TenantID) {
		return "", fmt.Errorf("Core returned an invalid verified tenant identity")
	}
	return owner.TenantID, nil
}
