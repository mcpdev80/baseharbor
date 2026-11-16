package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

func removeExternalProviderWithImpact(ctx context.Context, id string, in io.Reader, out io.Writer) (application.ExternalProviderRemovalResult, error) {
	root, err := bhruntime.DataDir("")
	if err != nil {
		return application.ExternalProviderRemovalResult{}, err
	}
	plan, err := application.PlanExternalProviderRemovalAt(root, id)
	if err != nil {
		return application.ExternalProviderRemovalResult{}, err
	}
	if err := authorizeExternalProviderRemovalScope(ctx, plan.Scope); err != nil {
		return application.ExternalProviderRemovalResult{}, err
	}
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return application.ExternalProviderRemovalResult{}, err
	}
	if out != nil {
		if plan.Scope.Attention != "" {
			fmt.Fprintln(out, "Achtung: review all Target bindings and the unknown live access consequences.")
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(plan.Scope); err != nil {
			return application.ExternalProviderRemovalResult{}, err
		}
	}
	if consent == nil && !noInput(ctx) && in != nil && readerIsTerminal(in) {
		// Reuse the same staged reader. Scope confirmation acknowledges the
		// aggregate ID; an individual Target ID never authorizes this removal.
		presentation := application.ProviderImpactPlan{PlanID: plan.Scope.PlanID, ConfirmationsRequired: plan.Scope.ConfirmationsRequired, Destructive: plan.Scope.Destructive}
		value, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, presentation)
		if err != nil {
			return application.ExternalProviderRemovalResult{}, err
		}
		consent = &value
	}
	if consent == nil {
		consent = &application.ProviderChangeConsent{PlanID: plan.Scope.PlanID}
	}
	if err := application.ValidateProviderImpactScopeConsent(plan.Scope, *consent); err != nil {
		return application.ExternalProviderRemovalResult{}, externalProviderScopeError(plan.Scope, err)
	}
	result, err := application.ApplyExternalProviderRemovalAt(root, id, *consent)
	if err != nil {
		classified := externalProviderScopeError(plan.Scope, err)
		var failure *machine.Error
		if errors.As(classified, &failure) {
			failure.Details = struct {
				Plan   application.ProviderImpactScope           `json:"plan"`
				Result application.ExternalProviderRemovalResult `json:"result"`
			}{plan.Scope, result}
		}
		return result, classified
	}
	return result, nil
}

func authorizeExternalProviderRemovalScope(ctx context.Context, scope application.ProviderImpactScope) error {
	// The existing global registration owner contains no tenant ownership.
	// A tenant role cannot manufacture authority over that global record.
	if _, scoped := tenancy.FromContext(ctx); scoped {
		return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "provider_registration_tenant_owner_unverifiable", Message: "The global external-provider registration has no verified tenant owner.", Next: "Use the verified local registration owner; tenant approval cannot grant authority over global registration state."}
	}
	seen := map[string]bool{}
	for _, target := range scope.Targets {
		for _, impact := range target.Impacts {
			for _, function := range impact.Functions {
				parts := strings.Split(function.Consumer, "/")
				if len(parts) != 3 || parts[0] != "application" || parts[2] == "" {
					return &machine.Error{Code: machine.ErrorOwnershipAmbiguous, CauseCode: "provider_consumer_authority_unverifiable", Message: "An affected provider consumer has no verified application/environment authority."}
				}
				key := target.Target + "/" + function.Consumer
				if seen[key] {
					continue
				}
				seen[key] = true
				if err := authorizeMCPOperation(ctx, "provider.remove", target.Target, parts[2], parts[1], ""); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func externalProviderScopeError(scope application.ProviderImpactScope, err error) error {
	err = classifyProviderChangeConsent(err)
	var classified *machine.Error
	if errors.As(err, &classified) {
		classified.Details = scope
	}
	return err
}
