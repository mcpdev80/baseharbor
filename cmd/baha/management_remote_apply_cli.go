package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func bindRemoteApplyConsent(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	consent, err := invocationProviderChangeConsent(ctx)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if consent != nil && len(fields["provider_change_consent"]) == 0 {
		fields["provider_change_consent"], err = json.Marshal(consent)
		if err != nil {
			return nil, err
		}
	}
	if digest := candidateConsentOverride(ctx); digest != "" && len(fields["confirm_candidate"]) == 0 {
		fields["confirm_candidate"], err = json.Marshal(digest)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

func remoteApplicationApplyCLI(ctx context.Context, target deployment.ResolvedTarget, args []string, out, errOut io.Writer) error {
	args, format, err := parseReadOutputArgs(args, "app apply")
	if err != nil {
		return err
	}
	if format == outputJSON {
		ctx = machineNoninteractiveContext(ctx)
	}
	var filtered []string
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			ctx = withAssumeYes(ctx, true)
		} else {
			filtered = append(filtered, arg)
		}
	}
	selectorArgs, idempotencyKey, err := extractRemoteIdempotencyKey(filtered)
	if err != nil {
		return err
	}
	name, environment, payload, err := remoteMutationArgs("apply", selectorArgs)
	if err != nil {
		return err
	}
	var supplied struct {
		ConfirmCandidate string                             `json:"confirm_candidate"`
		Consent          *application.ProviderChangeConsent `json:"provider_change_consent"`
	}
	if err := json.Unmarshal(payload, &supplied); err != nil {
		return err
	}
	ctx = withCandidateConsentDigest(ctx, supplied.ConfirmCandidate)
	root, m, matched, err := matchingRemoteRepository(name, environment)
	if err != nil {
		return err
	}
	var requested *application.PortableContract
	if matched {
		contract, err := application.PortableContractFromManifest(m)
		if err != nil {
			return err
		}
		requested = &contract
		if err := initializeRemoteRepository(ctx, target, root, m, repositoryInitOptions{}, errOut); err != nil {
			return err
		}
	}
	interactive := remoteCLIInteractive(ctx) && idempotencyKey == ""
	reader := bufio.NewReader(applicationInput(ctx, runtimeInput))
	seen := map[string]bool{}
	if supplied.Consent != nil {
		seen[supplied.Consent.PlanID] = true
	}
	for attempt := 0; attempt < 3; attempt++ {
		var result bytes.Buffer
		var requestErr error
		phase := "Deploy application on " + target.Name
		if candidateConsentOverride(ctx) == "" {
			phase = "Prepare deployment on " + target.Name
		}
		err := remoteCLIPhase(ctx, errOut, phase, func(ctx context.Context) error {
			requestErr = remoteApplicationMutation(ctx, target, "apply", filtered, &result)
			var challenge *machine.Error
			if errors.As(requestErr, &challenge) && challenge.Code == machine.ErrorApprovalRequired &&
				(challenge.CauseCode == "impact-confirmation-required" || challenge.CauseCode == "data-deletion-consent-required" || challenge.CauseCode == "candidate_confirmation_required") {
				reportRemoteProgress(ctx, "Installation plan ready.")
				return nil
			}
			return requestErr
		})
		if err == nil {
			err = requestErr
		}
		if err == nil {
			_, err = io.Copy(out, &result)
			return err
		}
		if !interactive {
			return err
		}
		next, retry, approvalErr := acknowledgeRemoteApplyChallenge(ctx, target, environment, err, requested, reader, errOut, seen)
		if approvalErr != nil || !retry {
			if approvalErr != nil {
				return approvalErr
			}
			return err
		}
		ctx = next
	}
	return errors.New("Core requires further authorization; inspect the current plan before retrying")
}

// Only explicit, read-only authorization challenges may start a new execution.
// Transport errors, pending executions, stale consent and partial failures never
// trigger a mutation retry. The Core revalidates every acknowledged plan/hash.
func acknowledgeRemoteApplyChallenge(ctx context.Context, target deployment.ResolvedTarget, environment string, cause error, requested *application.PortableContract, reader *bufio.Reader, out io.Writer, seen map[string]bool) (context.Context, bool, error) {
	var failure *machine.Error
	if !errors.As(cause, &failure) || failure.Code != machine.ErrorApprovalRequired {
		return ctx, false, nil
	}
	switch failure.CauseCode {
	case "impact-confirmation-required", "data-deletion-consent-required":
		payload, err := json.Marshal(failure.Details)
		if err != nil {
			return ctx, false, err
		}
		var plan application.ProviderImpactPlan
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&plan) != nil || plan.Version != application.ProviderImpactVersion ||
			plan.PlanID == "" || plan.Target == "" || len(plan.Impacts) == 0 ||
			plan.ConfirmationsRequired < 1 || plan.ConfirmationsRequired > 2 || seen[plan.PlanID] {
			return ctx, false, errors.New("Core returned an invalid or repeated provider confirmation plan; inspect before retrying")
		}
		core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
		if err != nil {
			return ctx, false, err
		}
		if plan.Target != core.serverTargetName() {
			return ctx, false, errors.New("provider confirmation plan belongs to another Core Target")
		}
		// Display the workstation alias; the server-side selector was verified above.
		displayPlan := plan
		displayPlan.Target = target.Name
		if err := printProviderImpactForCLI(ctx, out, displayPlan); err != nil {
			return ctx, false, err
		}
		digest := remoteChallengeCandidateDigest(failure.Next)
		if digest != "" {
			if seen["candidate"] || (candidateConsentOverride(ctx) != "" && candidateConsentOverride(ctx) != digest) {
				return ctx, false, errors.New("application source changed after confirmation; review the new plan")
			}
			fmt.Fprintln(out, "  Deploy application from the current source")
			if cli.OutputOptionsFromContext(ctx).Verbose {
				fmt.Fprintf(out, "  Source SHA-256: %s\n", digest)
			}
		}
		consent, err := readProviderApplyAcknowledgement(reader, out, plan)
		if errors.Is(err, io.EOF) || (err == nil && !consent.First) {
			return ctx, false, presentRemoteDeploymentCancellation(out, failure)
		}
		if err != nil {
			return ctx, false, err
		}
		if err := application.ValidateProviderChangeConsent(plan, consent); err != nil {
			return ctx, false, providerImpactConsentError(plan, err)
		}
		seen[plan.PlanID] = true
		if digest != "" {
			seen["candidate"] = true
			ctx = withCandidateConsentDigest(ctx, digest)
		}
		return withProviderChangeConsent(ctx, &consent), true, nil
	case "candidate_confirmation_required":
		match := regexp.MustCompile(` --confirm-candidate ([0-9a-f]{64})$`).FindStringSubmatch(failure.Next)
		if len(match) != 2 || seen["candidate"] || candidateConsentOverride(ctx) != "" {
			return ctx, false, nil
		}
		if err := printRemoteDeploymentChanges(ctx, target, environment, requested, failure.Details, out); err != nil {
			return ctx, false, err
		}
		if cli.OutputOptionsFromContext(ctx).Verbose {
			fmt.Fprintf(out, "  Source SHA-256: %s\n", match[1])
		}
		accepted, err := promptYesNo(reader, out, "Apply these changes?", false)
		if errors.Is(err, io.EOF) || (err == nil && !accepted) {
			return ctx, false, presentRemoteDeploymentCancellation(out, failure)
		}
		if err != nil {
			return ctx, false, err
		}
		seen["candidate"] = true
		return withCandidateConsentDigest(ctx, strings.TrimSpace(match[1])), true, nil
	}
	return ctx, false, nil
}

func remoteChallengeCandidateDigest(next string) string {
	match := regexp.MustCompile(` --confirm-candidate ([0-9a-f]{64})$`).FindStringSubmatch(next)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func presentRemoteDeploymentCancellation(out io.Writer, cause error) error {
	fmt.Fprintln(out, "Deployment cancelled; no rollout was started.")
	fmt.Fprintln(out, "To try again, run baha up from this application's repository.")
	return cli.Presented(cause)
}

func printRemoteDeploymentChanges(ctx context.Context, target deployment.ResolvedTarget, environment string, requested *application.PortableContract, details any, out io.Writer) error {
	if details == nil {
		fmt.Fprintln(out, "Service changes were not reported by this Core; existing services will be checked before deployment.")
		return nil
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	var plan applicationLifecyclePlan
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil || plan.ContractVersion != "v1" || plan.Environment != environment || plan.Footprint.Version != application.FootprintVersion || plan.ProviderPlan.Version != application.ProviderPlanVersion || plan.Footprint.Target != plan.ProviderPlan.Target || (requested != nil && plan.Application != requested.Application) {
		return errors.New("Core returned an invalid deployment preview; inspect the current plan before retrying")
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return err
	}
	if plan.Footprint.Target != core.serverTargetName() {
		return errors.New("deployment preview belongs to another Core Target")
	}
	printApplicationDeploymentChanges(out, target.Name, plan)
	return nil
}
