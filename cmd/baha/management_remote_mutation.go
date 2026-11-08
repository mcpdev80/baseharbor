package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// remoteMutationArgs accepts only options carried by the existing Machine HTTP
// contract. Unsupported local-only flags fail closed, rather than silently
// changing their meaning when a user selects a remote Target.
func remoteMutationArgs(operation string, args []string) (string, string, json.RawMessage, error) {
	filtered, environment, err := extractApplicationEnvironment(args, "app "+operation)
	if err != nil {
		return "", "", nil, err
	}
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" {
		return "", "", nil, usageError("remote application mutations currently require the dev operator session",
			"Use --environment dev; other environments require their own verified operator login.")
	}
	name, candidate, consentPath := "", "", ""
	approval, noBackup, fullReset, skipMemory := false, false, false, false
	for i := 0; i < len(filtered); i++ {
		arg := filtered[i]
		switch {
		case arg == "--yes":
			approval = true
		case arg == "--no-backup":
			noBackup = true
		case arg == "--full-reset":
			fullReset = true
		case arg == "--skip-memory-preflight":
			skipMemory = true
		case arg == "--provider-consent-file":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") || consentPath != "" {
				return "", "", nil, usageError("missing or duplicate provider consent file", "Pass one --provider-consent-file PATH containing an explicitly approved current plan.")
			}
			consentPath = filtered[i]
		case strings.HasPrefix(arg, "--provider-consent-file="):
			if consentPath != "" {
				return "", "", nil, usageError("duplicate provider consent file", "Pass only one current plan consent file.")
			}
			consentPath = strings.TrimPrefix(arg, "--provider-consent-file=")
		case arg == "--confirm-candidate":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") {
				return "", "", nil, usageError("missing candidate confirmation digest", "Pass --confirm-candidate DIGEST.")
			}
			candidate = filtered[i]
		case strings.HasPrefix(arg, "--confirm-candidate="):
			candidate = strings.TrimPrefix(arg, "--confirm-candidate=")
		default:
			if strings.HasPrefix(arg, "-") || name != "" {
				return "", "", nil, usageError("unsupported remote application option",
					"Use an explicit application NAME and only supported remote lifecycle flags.")
			}
			name = arg
		}
	}
	if strings.TrimSpace(name) == "" {
		return "", "", nil, usageError("remote application mutation needs an explicit NAME",
			"Use the application name already registered on the remote Core; the workstation source is not uploaded implicitly.")
	}
	var consent *application.ProviderChangeConsent
	if consentPath != "" {
		consent, err = loadRemoteProviderConsent(consentPath)
		if err != nil {
			return "", "", nil, err
		}
	}
	switch operation {
	case "apply":
		if approval || noBackup || fullReset {
			return "", "", nil, usageError("remote apply received unsupported lifecycle flags",
				"Use baha app apply NAME --environment dev [--skip-memory-preflight].")
		}
		payload, err := json.Marshal(struct {
			Name                  string                             `json:"name"`
			Environment           string                             `json:"environment"`
			SkipMemoryPreflight   bool                               `json:"skip_memory_preflight,omitempty"`
			ConfirmCandidate      string                             `json:"confirm_candidate,omitempty"`
			ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
		}{name, environment, skipMemory, candidate, consent})
		return name, environment, payload, err
	case "update":
		if !noBackup || approval || fullReset || skipMemory || candidate != "" || consent != nil {
			return "", "", nil, usageError("remote update requires explicit --no-backup and no unsupported flags",
				"Remote update with a backup requires a password file on the Core host; use baha app update NAME --environment dev --no-backup only if you accept that risk.")
		}
		payload, err := json.Marshal(struct {
			Environment string `json:"environment"`
			NoBackup    bool   `json:"no_backup"`
		}{environment, true})
		return name, environment, payload, err
	case "destroy":
		if !approval || noBackup || skipMemory || candidate != "" {
			return "", "", nil, usageError("remote destroy requires explicit --yes approval",
				"Use baha app destroy NAME --environment dev --yes [--full-reset]; --yes never replaces provider impact or data-deletion consent.")
		}
		payload, err := json.Marshal(struct {
			Name                  string                             `json:"name"`
			Environment           string                             `json:"environment"`
			Approval              bool                               `json:"approval"`
			FullReset             bool                               `json:"full_reset,omitempty"`
			ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
		}{name, environment, true, fullReset, consent})
		return name, environment, payload, err
	default:
		return "", "", nil, errors.New("unsupported remote lifecycle operation")
	}
}

func remoteApplicationMutation(ctx context.Context, target deployment.ResolvedTarget, operation string, args []string, out io.Writer) error {
	name, environment, payload, err := remoteMutationArgs(operation, args)
	if err != nil {
		return err
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return machine.Wrap(machine.ErrorAuthenticationFailed, err,
			"Configure and authenticate the pinned remote Core connection for this Target.", false)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate remote mutation request identity: %w", err)
	}
	selectors := machine.OperationContext{
		Application: name, Environment: environment, Target: target.Name,
	}
	admitted, err := core.requestMachineExecution(ctx, operation, selectors, payload, hex.EncodeToString(key))
	if err != nil {
		return err
	}
	// The Core may still be converging when the admission is received.
	// Poll only the read endpoint. NEVER issue another mutation on timeout.
	observationCtx, cancel := context.WithTimeout(ctx, 31*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var result json.RawMessage
	for {
		result, err = core.confirmRemoteExecutionSuccess(observationCtx, admitted)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "not terminal") {
			return err
		}
		select {
		case <-observationCtx.Done():
			return fmt.Errorf("remote Core execution %s not completed; inspect the existing execution before retrying, do not submit another mutation: %w",
				admitted.ExecutionID, observationCtx.Err())
		case <-ticker.C:
		}
	}
	if !json.Valid(result) {
		return errors.New("remote Core lifecycle result is not valid JSON")
	}
	// Mutating execution results must remain scoped to the requested app.
	// Apply/update return a status object, destroy returns a scoped retirement.
	var observed struct {
		Target      string `json:"target"`
		Application string `json:"application"`
		Environment string `json:"environment"`
		Status      *struct {
			Target      string `json:"target"`
			Application string `json:"application"`
			Environment           string `json:"environment"`
			Ready       bool   `json:"ready"`
		} `json:"status,omitempty"`
	}
	if err := json.Unmarshal(result, &observed); err != nil {
		return errors.New("remote Core lifecycle result cannot be decoded")
	}
	if observed.Target != target.Name || observed.Application != name || observed.Environment != environment {
		return errors.New("remote Core lifecycle result does not match the requested application authority")
	}
	if operation == "apply" && (observed.Status == nil || !observed.Status.Ready) {
		return errors.New("remote Core apply did not prove application readiness")
	}
	if observed.Status != nil && (observed.Status.Target != target.Name ||
		observed.Status.Application != name || observed.Status.Environment != environment) {
		return errors.New("remote Core lifecycle status belongs to another application authority")
	}
	return writeJSON(out, result)
}

func loadRemoteProviderConsent(path string) (*application.ProviderChangeConsent, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("remote provider consent file path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open explicit provider consent file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return nil, errors.New("provider consent must be a regular JSON file of at most 16 KiB")
	}
	var consent application.ProviderChangeConsent
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&consent); err != nil || decoder.Decode(new(any)) != io.EOF ||
		strings.TrimSpace(consent.PlanID) == "" || !consent.First || !consent.Second {
		return nil, errors.New("provider consent must explicitly acknowledge both stages of one current plan")
	}
	return &consent, nil
}
