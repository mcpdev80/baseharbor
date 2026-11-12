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
	filtered, workloadPorts, err := extractRemoteWorkloadPorts(filtered)
	if err != nil {
		return "", "", nil, err
	}
	name, candidate, consentPath := "", "", ""
	hostname, tlsMode, certificateDirectory := "", "", ""
	candidateSeen, consentSeen := false, false
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
		case arg == "--hostname":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") || hostname != "" {
				return "", "", nil, usageError("missing or duplicate hostname", "Pass one --hostname HOST value.")
			}
			hostname = filtered[i]
		case strings.HasPrefix(arg, "--hostname="):
			if hostname != "" {
				return "", "", nil, usageError("duplicate hostname", "Pass --hostname once.")
			}
			hostname = strings.TrimPrefix(arg, "--hostname=")
		case arg == "--tls":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") || tlsMode != "" {
				return "", "", nil, usageError("missing or duplicate TLS mode", "Pass one --tls acme|local|existing value.")
			}
			tlsMode = filtered[i]
		case strings.HasPrefix(arg, "--tls="):
			if tlsMode != "" {
				return "", "", nil, usageError("duplicate TLS mode", "Pass --tls once.")
			}
			tlsMode = strings.TrimPrefix(arg, "--tls=")
		case arg == "--cert-dir":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") || certificateDirectory != "" {
				return "", "", nil, usageError("missing or duplicate certificate directory", "Pass one --cert-dir PATH value.")
			}
			certificateDirectory = filtered[i]
		case strings.HasPrefix(arg, "--cert-dir="):
			if certificateDirectory != "" {
				return "", "", nil, usageError("duplicate certificate directory", "Pass --cert-dir once.")
			}
			certificateDirectory = strings.TrimPrefix(arg, "--cert-dir=")
		case arg == "--provider-consent-file":
			i++
			if i >= len(filtered) || strings.HasPrefix(filtered[i], "-") || consentSeen {
				return "", "", nil, usageError("missing or duplicate provider consent file", "Pass one --provider-consent-file PATH containing an explicitly approved current plan.")
			}
			consentPath = filtered[i]
			consentSeen = true
		case strings.HasPrefix(arg, "--provider-consent-file="):
			if consentSeen {
				return "", "", nil, usageError("duplicate provider consent file", "Pass only one current plan consent file.")
			}
			consentPath = strings.TrimPrefix(arg, "--provider-consent-file=")
			consentSeen = true
		case arg == "--confirm-candidate":
			i++
			if candidateSeen || i >= len(filtered) || strings.HasPrefix(filtered[i], "-") {
				return "", "", nil, usageError("missing candidate confirmation digest", "Pass --confirm-candidate DIGEST.")
			}
			candidate = filtered[i]
			candidateSeen = true
		case strings.HasPrefix(arg, "--confirm-candidate="):
			if candidateSeen {
				return "", "", nil, usageError("duplicate candidate confirmation", "Supply --confirm-candidate DIGEST exactly once.")
			}
			candidate = strings.TrimPrefix(arg, "--confirm-candidate=")
			candidateSeen = true
		default:
			if strings.HasPrefix(arg, "-") || name != "" {
				return "", "", nil, usageError("unsupported remote application option",
					"Use an explicit application NAME and only supported remote lifecycle flags.")
			}
			name = arg
		}
	}
	if consentSeen && strings.TrimSpace(consentPath) == "" {
		return "", "", nil, usageError("empty provider consent file", "Pass a non-empty --provider-consent-file PATH containing the approved current plan.")
	}
	if candidateSeen && strings.TrimSpace(candidate) == "" {
		return "", "", nil, usageError("empty candidate confirmation digest", "Pass the exact candidate SHA-256 digest or omit --confirm-candidate.")
	}
	name, environment, err = remoteMutationSelector(name, environment)
	if err != nil {
		return "", "", nil, err
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
		if candidate != "" {
			if len(candidate) != 64 || strings.ToLower(candidate) != candidate {
				return "", "", nil, usageError("invalid candidate confirmation digest", "Provide the exact 64-character lowercase SHA-256 digest displayed by the candidate validation.")
			}
			if _, err := hex.DecodeString(candidate); err != nil {
				return "", "", nil, usageError("invalid candidate confirmation digest", "Provide the exact lowercase SHA-256 digest displayed by the candidate validation.")
			}
		}
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
			WorkloadPorts         map[string]string                  `json:"workload_ports,omitempty"`
		}{name, environment, skipMemory, candidate, consent, workloadPorts})
		return name, environment, payload, err
	case "update":
		if !noBackup || approval || fullReset || skipMemory || candidate != "" || consent != nil || len(workloadPorts) > 0 {
			return "", "", nil, usageError("remote update requires explicit --no-backup and no unsupported flags",
				"Remote update with a backup requires a password file on the Core host; use baha app update NAME --environment dev --no-backup only if you accept that risk.")
		}
		payload, err := json.Marshal(struct {
			Environment string `json:"environment"`
			NoBackup    bool   `json:"no_backup"`
		}{environment, true})
		return name, environment, payload, err
	case "app.stop":
		if approval || noBackup || fullReset || skipMemory || candidate != "" || len(workloadPorts) > 0 {
			return "", "", nil, usageError("remote stop received unsupported lifecycle flags",
				"Use baha app down NAME --environment dev [--provider-consent-file PATH].")
		}
		payload, err := json.Marshal(struct {
			Name                  string                             `json:"name"`
			Environment           string                             `json:"environment"`
			ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
		}{name, environment, consent})
		return name, environment, payload, err
	case "destroy":
		if !approval || noBackup || skipMemory || candidate != "" || len(workloadPorts) > 0 {
			return "", "", nil, usageError("remote destroy requires explicit --yes approval",
				"Run baha app destroy NAME --yes; the current provider plan will be shown before removal.")
		}
		payload, err := json.Marshal(struct {
			Name                  string                             `json:"name"`
			Environment           string                             `json:"environment"`
			Approval              bool                               `json:"approval"`
			FullReset             bool                               `json:"full_reset,omitempty"`
			ProviderChangeConsent *application.ProviderChangeConsent `json:"provider_change_consent,omitempty"`
		}{name, environment, true, fullReset, consent})
		return name, environment, payload, err
	case "app.configure":
		if candidate != "" || consent != nil || noBackup || fullReset || skipMemory || certificateDirectory != "" || len(workloadPorts) > 0 {
			return "", "", nil, usageError("remote app init received an unsupported option", "Use baha app init --tls local on a remote development Target; certificate paths on the workstation cannot be read by the Core host.")
		}
		if tlsMode == "existing" {
			return "", "", nil, usageError("remote existing-certificate initialization is unavailable", "Install or securely import the certificate on the Core host before selecting TLS mode existing.")
		}
		payload, err := json.Marshal(machineApplicationConfigureInput{
			Name: name, Environment: environment, Hostname: hostname, TLSMode: tlsMode,
		})
		return name, environment, payload, err
	default:
		return "", "", nil, errors.New("unsupported remote lifecycle operation")
	}
}

func extractRemoteWorkloadPorts(args []string) ([]string, map[string]string, error) {
	filtered := make([]string, 0, len(args))
	ports := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := ""
		if arg == "--workload-port" {
			i++
			if i >= len(args) {
				return nil, nil, usageError("missing workload port", "Pass --workload-port NAME=PORT.")
			}
			value = args[i]
		} else if strings.HasPrefix(arg, "--workload-port=") {
			value = strings.TrimPrefix(arg, "--workload-port=")
		} else {
			filtered = append(filtered, arg)
			continue
		}
		name, port, ok := strings.Cut(value, "=")
		if !ok || !validWorkloadEnvironmentName(name) || strings.TrimSpace(port) != port {
			return nil, nil, usageError("invalid workload port", "Pass --workload-port NAME=PORT with a valid environment name and port.")
		}
		if _, duplicate := ports[name]; duplicate {
			return nil, nil, usageError("duplicate workload port", "Pass each --workload-port NAME=PORT once.")
		}
		if _, err := parsePort(port); err != nil {
			return nil, nil, usageError("invalid workload port", "Choose a TCP port from 1 through 65535.")
		}
		ports[name] = port
	}
	return filtered, ports, nil
}

func remoteApplicationMutation(ctx context.Context, target deployment.ResolvedTarget, operation string, args []string, out io.Writer) error {
	filteredArgs, explicitKey, err := extractRemoteIdempotencyKey(args)
	if err != nil {
		return err
	}
	name, environment, payload, err := remoteMutationArgs(operation, filteredArgs)
	if err != nil {
		return err
	}
	if operation == "apply" || operation == "destroy" {
		payload, err = bindRemoteApplyConsent(ctx, payload)
		if err != nil {
			return err
		}
	}
	core, err := dialVerifiedRemoteCore(ctx, target.Name, target.RuntimeProvider, target.TenantID)
	if err != nil {
		return machine.Wrap(machine.ErrorAuthenticationFailed, err,
			"Configure and authenticate the pinned remote Core connection for this Target.", false)
	}
	if operation == "apply" || operation == "app.configure" {
		reportRemoteProgress(ctx, "Transferring the selected application source to the Core.")
		root, rootErr := remoteLocalSourceRoot(name, environment)
		if rootErr != nil {
			return rootErr
		}
		if root != "" {
			digest, transferErr := core.uploadLocalSource(ctx, target.Name, name, environment, root)
			if transferErr != nil {
				return transferErr
			}
			payload, err = remotePayloadWithSnapshot(payload, digest)
		} else {
			sourceModel, sourceErr := selectedRemoteSourceModel(name, environment)
			if sourceErr != nil {
				return sourceErr
			}
			payload, err = remoteApplyPayloadWithSource(payload, sourceModel)
		}
		if err != nil {
			return err
		}
	}
	key := explicitKey
	if key == "" {
		randomKey := make([]byte, 16)
		if _, err := rand.Read(randomKey); err != nil {
			return fmt.Errorf("generate remote mutation request identity: %w", err)
		}
		key = hex.EncodeToString(randomKey)
	}
	selectors := machine.OperationContext{
		Application: name, Environment: environment, Target: target.Name,
	}
	admitted, err := core.requestMachineExecution(ctx, operation, selectors, payload, key)
	if err != nil {
		return fmt.Errorf("remote operation %s/%s request key %s (retain this key and use --idempotency-key for an exact retry): %w",
			target.Name, name, key, err)
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
		if !errors.Is(err, errRemoteExecutionPending) {
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
		ContractVersion string `json:"contract_version"`
		Operation       string `json:"operation"`
		Verified        bool   `json:"verified"`
		Target          string `json:"target"`
		Application     string `json:"application"`
		Environment     string `json:"environment"`
		Status          *struct {
			Target      string `json:"target"`
			Application string `json:"application"`
			Environment string `json:"environment"`
			Ready       bool   `json:"ready"`
		} `json:"status,omitempty"`
	}
	if err := json.Unmarshal(result, &observed); err != nil {
		return errors.New("remote Core lifecycle result cannot be decoded")
	}
	if operation != "app.stop" && (observed.ContractVersion != machine.ContractVersion || observed.Operation != operation || !observed.Verified) {
		return errors.New("remote Core lifecycle receipt is unverified or does not match the requested operation")
	}
	if (operation != "app.stop" && observed.Target != target.Name) || observed.Application != name || observed.Environment != environment {
		return errors.New("remote Core lifecycle result does not match the requested application authority")
	}
	if operation == "apply" && (observed.Status == nil || !observed.Status.Ready) {
		return errors.New("remote Core apply did not prove application readiness")
	}
	if operation == "app.stop" {
		var stopped struct {
			State         string `json:"state"`
			DataPreserved bool   `json:"data_preserved"`
		}
		if err := json.Unmarshal(result, &stopped); err != nil ||
			stopped.State != "stopped" || !stopped.DataPreserved {
			return errors.New("remote Core did not verify application stop and retained recovery ownership")
		}
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

func extractRemoteIdempotencyKey(args []string) ([]string, string, error) {
	filtered := make([]string, 0, len(args))
	var key string
	seen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--idempotency-key":
			i++
			if i >= len(args) || seen {
				return nil, "", usageError("missing or duplicate idempotency key", "Pass --idempotency-key KEY exactly once.")
			}
			key = args[i]
			seen = true
		case strings.HasPrefix(arg, "--idempotency-key="):
			if seen {
				return nil, "", usageError("duplicate idempotency key", "Pass one idempotency key only.")
			}
			key = strings.TrimPrefix(arg, "--idempotency-key=")
			seen = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if seen && !remoteExecutionIDPattern.MatchString(key) {
		return nil, "", usageError("invalid idempotency key", "Use 1-128 ASCII letters, numbers, '_' or '-', starting with a letter or number.")
	}
	return filtered, key, nil
}
