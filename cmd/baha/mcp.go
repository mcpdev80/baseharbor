package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

type machineProviderIDInput struct {
	ID string `json:"id" jsonschema:"registered external provider id"`
}

type machineProviderAddInput struct {
	ID                string   `json:"id" jsonschema:"external provider registration id"`
	ProviderID        string   `json:"provider_id" jsonschema:"provider descriptor id such as company/postgresql"`
	ProviderVersion   string   `json:"provider_version,omitempty" jsonschema:"optional provider descriptor version"`
	ProviderProtocol  string   `json:"provider_protocol,omitempty" jsonschema:"optional provider protocol version"`
	Kind              string   `json:"kind" jsonschema:"provider implementation kind"`
	Capabilities      []string `json:"capabilities" jsonschema:"portable BaseHarbor capabilities implemented by this provider"`
	Endpoint          string   `json:"endpoint" jsonschema:"absolute application-facing provider endpoint without embedded credentials"`
	CredentialRef     string   `json:"credential_ref,omitempty" jsonschema:"secret-safe credential reference; plaintext credentials are not accepted"`
	TrustMode         string   `json:"trust_mode,omitempty" jsonschema:"system, custom-ca or mtls; defaults to system"`
	CAReference       string   `json:"ca_reference,omitempty" jsonschema:"custom CA/trust reference when required"`
	ClientCertificate string   `json:"client_certificate_reference,omitempty" jsonschema:"mTLS client certificate reference"`
	ClientKey         string   `json:"client_key_reference,omitempty" jsonschema:"mTLS client private-key reference; key material is never accepted directly"`
	CertificateDir    string   `json:"certificate_directory,omitempty" jsonschema:"directory containing BYOC certificate/trust material for safe discovery"`
}

type machineProviderRemoveInput struct {
	ID       string `json:"id" jsonschema:"registered external provider id"`
	Approval bool   `json:"approval,omitempty" jsonschema:"explicit approval required to remove BaseHarbor registration; foreign infrastructure is never destroyed"`
}

type machineOrganizationInput struct {
	Environment string `json:"environment,omitempty" jsonschema:"environment used to resolve effective organization defaults; defaults to dev"`
}

type machineOrganizationSetInput struct {
	Source      string `json:"source" jsonschema:"organization source kind: oci, git, local or system"`
	Location    string `json:"location,omitempty" jsonschema:"OCI repository, Git repository, local path, or managed system path"`
	Requested   string `json:"requested,omitempty" jsonschema:"requested OCI tag/channel or Git ref; the resolved immutable digest/revision is persisted"`
	Environment string `json:"environment,omitempty" jsonschema:"environment used to return effective defaults; defaults to dev"`
}

type machineOrganizationUpdateInput struct {
	Approval    bool   `json:"approval,omitempty" jsonschema:"explicit approval required after reviewing organization.check"`
	Environment string `json:"environment,omitempty" jsonschema:"environment used to return effective defaults; defaults to dev"`
}

type machineTargetInput struct {
	Target string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
}

type machineInspectInput struct {
	Path string `json:"path,omitempty" jsonschema:"local repository path or Git URL; defaults to the current directory"`
}

type machineWorkspaceResolveInput struct {
	Manifest string `json:"manifest,omitempty" jsonschema:"canonical baseharbor.yaml path or directory containing it; defaults to the current repository"`
}

type machineWorkspaceStatusInput struct {
	Manifest string `json:"manifest,omitempty" jsonschema:"canonical baseharbor.yaml path or directory containing it; defaults to the current repository"`
	Fetch    bool   `json:"fetch,omitempty" jsonschema:"fetch the configured upstream before reporting status; read-only for the worktree"`
}

type machineWorkspaceUpdateInput struct {
	Manifest string `json:"manifest,omitempty" jsonschema:"canonical baseharbor.yaml path or directory containing it; defaults to the current repository"`
	Check    bool   `json:"check,omitempty" jsonschema:"fetch and report safe updates without changing checked-out revisions"`
}

type machineApplicationInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
}

type machineApplyInput struct {
	Target              string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target; otherwise uses BASEHARBOR_TARGET or configured default-target"`
	Name                string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment         string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	SkipMemoryPreflight bool   `json:"skip_memory_preflight,omitempty" jsonschema:"explicit approval to continue when host memory headroom is TIGHT; does not bypass hard memory failures"`
}

type machineAppNewInput struct {
	Path               string   `json:"path,omitempty" jsonschema:"deprecated compatibility field: exact empty project root; prefer directory for new clients"`
	Directory          string   `json:"directory,omitempty" jsonschema:"parent directory in which BaseHarbor creates a child directory named after the application; required unless the current empty directory already matches the application name"`
	Name               string   `json:"name" jsonschema:"application name"`
	Environment        string   `json:"environment,omitempty" jsonschema:"application environment; defaults to dev"`
	Stack              string   `json:"stack,omitempty" jsonschema:"built-in development stack; defaults to go when stack_profile is omitted"`
	StackProfile       string   `json:"stack_profile,omitempty" jsonschema:"reusable Stack Profile name from the effective built-in/user/repository catalog; mutually exclusive with stack"`
	Capabilities       []string `json:"capabilities,omitempty" jsonschema:"portable capability names such as exposure.http, database.sql, cache.key-value, database.key-value, database.document, messaging.queue, messaging.pubsub, messaging.stream, object-storage.s3, secrets, telemetry.otlp"`
	Secrets            []string `json:"secrets,omitempty" jsonschema:"required application secret binding names; values are never accepted"`
	EmitBackstage      bool     `json:"emit_backstage,omitempty" jsonschema:"emit a static Backstage catalog-info.yaml projection; default false"`
	BackstageOwner     string   `json:"backstage_owner,omitempty" jsonschema:"explicit Backstage owner when catalog emission is enabled; BaseHarbor never infers ownership"`
	BackstageLifecycle string   `json:"backstage_lifecycle,omitempty" jsonschema:"optional Backstage lifecycle; defaults to experimental"`
}

type machineUpdateInput struct {
	Target             string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Environment        string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	BackupPasswordFile string `json:"backup_password_file,omitempty" jsonschema:"owner-only local file containing the backup password used for the pre-update recovery point"`
	NoBackup           bool   `json:"no_backup,omitempty" jsonschema:"explicitly acknowledge updating durable state without a pre-update recovery point"`
}

type machineBackupInput struct {
	Target       string   `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Name         string   `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment  string   `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	OutputPath   string   `json:"output_path,omitempty" jsonschema:"optional local path for the encrypted BaseHarbor recovery archive"`
	PasswordFile string   `json:"password_file,omitempty" jsonschema:"owner-only local file containing the backup password; secret values are never accepted directly"`
	IncludeState []string `json:"include_state,omitempty" jsonschema:"optional typed recovery state classes to include"`
	ExcludeState []string `json:"exclude_state,omitempty" jsonschema:"optional typed recovery state classes to exclude from this partial recovery unit"`
}

type machineRestoreInput struct {
	Target       string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	BackupPath   string `json:"backup_path,omitempty" jsonschema:"local encrypted BaseHarbor recovery archive to restore"`
	Name         string `json:"name,omitempty" jsonschema:"optional expected application identity"`
	Environment  string `json:"environment,omitempty" jsonschema:"optional expected deployment environment"`
	PasswordFile string `json:"password_file" jsonschema:"owner-only local file containing the backup password; secret values are never accepted directly"`
}

type machineDestroyInput struct {
	Target      string `json:"target,omitempty" jsonschema:"optional BaseHarbor deployment target"`
	Name        string `json:"name,omitempty" jsonschema:"optional stored application name; omit inside an application repository"`
	Environment string `json:"environment,omitempty" jsonschema:"optional deployment environment selected from repository intent"`
	Approval    bool   `json:"approval,omitempty" jsonschema:"explicit operator approval required before destructive mutation"`
	FullReset   bool   `json:"full_reset,omitempty" jsonschema:"also remove BaseHarbor-owned repository deployment and normalized TLS state"`
}

type machineToolError struct {
	ContractVersion string         `json:"contract_version"`
	Error           *machine.Error `json:"error"`
}

type machineLifecycleStatusResult struct {
	applicationlifecycle.Result
	Status applicationStatusResult `json:"status"`
}

type machineLifecycleDoctorResult struct {
	applicationlifecycle.Result
	Doctor applicationDoctorResult `json:"doctor"`
}

type machineObserveResult struct {
	ContractVersion string                  `json:"contract_version"`
	Target          string                  `json:"target"`
	Application     string                  `json:"application"`
	Environment     string                  `json:"environment"`
	Status          applicationStatusResult `json:"status"`
	Doctor          applicationDoctorResult `json:"doctor"`
}

type machineBackupResult struct {
	applicationlifecycle.Result
	Backup application.BackupMetadata `json:"backup"`
}

func mcpCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "mcp",
		Summary: "Expose BaseHarbor semantic operations over MCP",
		Usage:   "baha mcp serve",
		Long:    "Runs a local stdio Model Context Protocol server exposing a deliberately small BaseHarbor semantic tool surface. It never exposes generic shell, Docker, Compose or Podman execution.",
		Children: []*cli.Command{
			{
				Name:    "serve",
				Summary: "Run the local stdio MCP server",
				Usage:   "baha mcp serve",
				Long:    "Uses stdin/stdout only. stdout is reserved exclusively for MCP protocol messages; diagnostics are returned as structured tool errors or written to stderr by the process entrypoint.",
				Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
					if len(args) != 0 {
						return usageError("baha mcp serve does not accept arguments", "Run 'baha mcp serve'.")
					}
					return runMCPServer(ctx, store)
				},
			},
		},
	}
}

func runMCPServer(ctx context.Context, store application.Store) error {
	return newMCPServer(store).Run(ctx, &mcp.StdioTransport{})
}

func newMCPServer(store application.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "baseharbor",
		Version: version,
	}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2026-07-28", "2025-11-25"},
		Capabilities:              &mcp.ServerCapabilities{},
	})

	registerMCPReadTools(server, store)
	registerMCPRuntimeExplorerMutationTools(server)
	registerMCPLifecycleTools(server, store)
	registerMCPWorkspaceMutationTools(server)
	return server
}

func machineMCPTool(operationID, description string, openWorld bool) *mcp.Tool {
	operation, ok := machine.OperationByID(operationID)
	if !ok || operation.MCPTool == "" {
		panic("BaseHarbor machine operation is not registered for MCP: " + operationID)
	}
	readOnly := operation.Safety == machine.SafetyReadOnly
	destructive := operation.Safety == machine.SafetyDestructive
	return &mcp.Tool{
		Name:        operation.MCPTool,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: boolPointer(destructive),
			OpenWorldHint:   boolPointer(openWorld),
		},
	}
}

func authorizeMCPOperation(ctx context.Context, operationID, target, environment, applicationName, workspace string) error {
	operation, ok := machine.OperationByID(operationID)
	if !ok {
		return machine.NewError(machine.ErrorValidationFailed, "Unknown BaseHarbor machine operation.", "Use a registered machine operation.", false)
	}
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment == "" {
		environment = "dev"
	}
	target = strings.TrimSpace(target)
	if target == "" {
		if resolved, err := effectiveTarget(ctx); err == nil {
			target = resolved.Name
		}
	}
	if operatorauth.ManagedEnvironment(environment) {
		if err := ensureOperatorAuthForBoundary(ctx, target, environment); err != nil {
			return &machine.Error{
				Code:        machine.ErrorAuthenticationFailed,
				CauseCode:   "operator_authentication_required",
				Message:     "Managed-environment machine operations require an authenticated BaseHarbor operator.",
				Resource:    strings.TrimSpace(target) + "/" + environment,
				Remediation: "authenticate the BaseHarbor operator",
				Next:        "Configure operator OIDC if required, run 'baha login' for the selected Target/environment, then retry.",
				Cause:       err,
			}
		}
	}
	_, err := operatorauth.AuthorizeMachineOperation(ctx, operatorauth.AuthorizationRequest{
		Operation: operation,
		Context: operatorauth.OperationContext{
			Application: strings.TrimSpace(applicationName),
			Environment: environment,
			Target:      target,
			Workspace:   strings.TrimSpace(workspace),
		},
	})
	return err
}

func authorizeResolvedMCPOperation(ctx context.Context, operationID string, resolved resolvedApplication, workspace string) error {
	return authorizeMCPOperation(
		ctx,
		operationID,
		resolved.Target.Name,
		resolved.Manifest.Environment,
		resolved.Manifest.Name,
		workspace,
	)
}

func authorizeCurrentMCPContext(ctx context.Context, operationID, targetName, environment, workspace string) error {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment == "" {
		environment = "dev"
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			if selection, selectionErr := application.ResolveRepositoryEnvironment(cwd, ""); selectionErr == nil {
				environment = selection.Manifest.Environment
			}
		}
	}

	targetName = strings.TrimSpace(targetName)
	if targetName == "" && !operatorauth.ManagedEnvironment(environment) {
		return authorizeMCPOperation(ctx, operationID, "", environment, "", workspace)
	}

	ctx = withTargetOverride(ctx, targetName)
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	return authorizeMCPOperation(ctx, operationID, target.Name, environment, "", workspace)
}

func machineApplicationArgs(name, environment string) []string {
	args := make([]string, 0, 3)
	if name = strings.TrimSpace(name); name != "" {
		args = append(args, name)
	}
	if environment = strings.TrimSpace(environment); environment != "" {
		args = append(args, "--environment", environment)
	}
	return args
}

const machineLifecycleMaxDuration = 30 * time.Minute

func machineLifecycleContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// Once a mutating MCP lifecycle operation has been accepted, convergence
	// must not be truncated merely because the client request times out or
	// disconnects. Preserve request values, detach client cancellation, then
	// apply a server-owned upper bound so an accepted operation cannot run
	// forever if an external runtime command wedges.
	detached := context.WithoutCancel(evidence.WithActor(ctx, "mcp", "local-agent"))
	bounded, cancel := context.WithTimeout(detached, machineLifecycleMaxDuration)
	opts := cli.OutputOptionsFromContext(bounded)
	opts.NonInteractive = true
	opts.Quiet = true
	opts.Plain = true
	return cli.WithOutputOptions(bounded, opts), cancel
}
func machineMCPFailure(err error) (*mcp.CallToolResult, any, error) {
	classified := machine.Classify(classifyMachineCLIError(err))
	payload := machineToolError{ContractVersion: machine.ContractVersion, Error: classified}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, payload, nil
}

func boolPointer(value bool) *bool {
	return &value
}
