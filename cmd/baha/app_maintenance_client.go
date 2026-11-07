package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// maintenanceOperations are the lifecycle operations this client can submit.
// Registering the human command and the machine discovery entry belongs to
// t-20261018-r27-public-surfaces; this slice owns only the routing.
var maintenanceOperations = application.MaintenanceOperations

// applicationMaintenanceCommand submits one maintenance operation for the
// selected application through the installation authority that owns its
// deployment. Every path a client names is a client-local artifact reference,
// never a path on the Target host.
func applicationMaintenanceCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "maintenance",
		Summary: "Submit one maintenance operation through the selected authority",
		Usage:   "baha app maintenance [NAME] --operation backup|restore|update|destroy [--artifact FILE] [--password-file FILE] [--no-backup] [--full-reset] [--yes]",
		Long:    "Routes update, backup, restore and destroy through the installation authority that owns the selected deployment and its existing lifecycle and consent owners. Archive and password-file references are explicit client-local safe channels, never a Target-host path. Maintenance intent without a qualified remote owner is rejected before any transport or mutation.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationMaintenance(ctx, store, args, out, errOut)
		},
	}
}

type applicationMaintenanceOptions struct {
	Name         string
	Operation    string
	Artifact     string
	PasswordFile string
	NoBackup     bool
	FullReset    bool
	Confirmed    bool
	Now          time.Time
}

func parseApplicationMaintenanceArgs(args []string) (applicationMaintenanceOptions, error) {
	options := applicationMaintenanceOptions{Now: time.Now().UTC()}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--operation" || arg == "--artifact" || arg == "--password-file":
			value, err := maintenanceFlagValue(args, i, arg)
			if err != nil {
				return options, err
			}
			i++
			switch arg {
			case "--operation":
				options.Operation = value
			case "--artifact":
				options.Artifact = value
			default:
				options.PasswordFile = value
			}
		case arg == "--no-backup" || arg == "--full-reset" || arg == "--yes" || arg == "-y":
			switch arg {
			case "--no-backup":
				options.NoBackup = true
			case "--full-reset":
				options.FullReset = true
			default:
				options.Confirmed = true
			}
		case strings.HasPrefix(arg, "-"):
			return options, maintenanceUsage("unknown option " + arg)
		default:
			if options.Name != "" {
				return options, maintenanceUsage("only one application name is accepted")
			}
			options.Name = arg
		}
	}
	if options.Operation == "" {
		return options, maintenanceUsage("--operation is required: " + strings.Join(maintenanceOperations, "|"))
	}
	if !application.IsMaintenanceOperation(options.Operation) {
		return options, maintenanceUsage("unknown maintenance operation " + options.Operation)
	}
	return options, nil
}

func maintenanceFlagValue(args []string, i int, flag string) (string, error) {
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
		return "", maintenanceUsage(flag + " requires a value")
	}
	return args[i+1], nil
}

func maintenanceUsage(message string) error {
	return &cli.UsageError{Message: message, Hint: "Use baha app maintenance [NAME] --operation backup|restore|update|destroy"}
}

func (o applicationMaintenanceOptions) applicationArgs() []string {
	if o.Name == "" {
		return nil
	}
	return []string{o.Name}
}

// executeApplicationMaintenance resolves the selected deployment once through
// the same application authority every other lifecycle path uses, then either
// routes the operation to the owning installation or refuses it explicitly.
func executeApplicationMaintenance(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	options, err := parseApplicationMaintenanceArgs(args)
	if err != nil {
		return err
	}
	// The name a client passes is only a selector. The deployment the operation
	// applies to is the one the repository checkout resolves to, so a submission
	// can never drift onto a different deployment than the caller is standing in.
	resolved, err := resolveApplication(ctx, store, nil, "app maintenance")
	if err != nil {
		return err
	}
	if options.Name != "" && options.Name != resolved.Manifest.Name {
		return machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("The application selector %q does not match the deployment resolved from the selected authority.", options.Name),
			"Run maintenance inside the intended repository checkout or omit the selector.", false)
	}
	return submitApplicationMaintenance(ctx, resolved, options, out, errOut)
}

func submitApplicationMaintenance(ctx context.Context, resolved resolvedApplication,
	options applicationMaintenanceOptions, out, errOut io.Writer) error {
	if !application.IsMaintenanceOperation(options.Operation) {
		return applicationMaintenanceUnknownOperation(options.Operation)
	}
	// An already cancelled submission stops before it reads any input, opens any
	// transport or writes any durable window.
	if !maintenanceSubmissionAllowed(ctx) {
		return maintenanceSubmissionCancelled()
	}
	if !hasRemoteApplicationTarget(resolved) {
		// The authority binding comes first on the local home: a deployment nobody
		// owns must be refused before any client input is opened.
		if _, err := applicationMaintenanceAuthority(resolved); err != nil {
			return err
		}
		// Path channels are validated before anything is transferred, so an unsafe
		// reference cannot reach a lifecycle owner.
		if err := validateMaintenanceClientChannels(options); err != nil {
			return err
		}
		return runApplicationMaintenanceOperation(ctx, resolved, options, out, errOut)
	}
	return submitRemoteApplicationMaintenance(ctx, resolved, options, out, errOut)
}

// maintenanceMutationCompleted reports whether a lifecycle owner actually
// converged the operation, the only condition under which a durable maintenance
// window may advance. Cancellation is checked again here so an operation that
// stopped mid-flight records nothing.
func maintenanceMutationCompleted(ctx context.Context, err error) bool {
	return err == nil && !maintenanceContextDone(ctx)
}

// maintenanceSubmissionAllowed is the single pre-flight gate every maintenance
// submission crosses: the caller's own cancellation must stop the submission
// before it reads anything, and a submission whose caller has gone must not
// record a window it can no longer stand behind.
func maintenanceSubmissionAllowed(ctx context.Context) bool {
	return !maintenanceContextDone(ctx)
}

func maintenanceSubmissionCancelled() error {
	return machine.NewError(machine.ErrorConflict,
		"The maintenance submission was cancelled before it reached the selected authority.",
		"Inspect the deployment state, then re-submit the operation you intended.", false)
}

// validateMaintenanceClientChannels admits only client-local safe channels. A
// Target-host path is never a valid maintenance input, so the client refuses a
// relative filename, an unclean spelling and any repository-internal path.
func validateMaintenanceClientChannels(options applicationMaintenanceOptions) error {
	if strings.TrimSpace(options.Artifact) != "" {
		if _, err := requireClientLocalMaintenanceArtifact(options.Artifact); err != nil {
			return err
		}
	}
	if strings.TrimSpace(options.PasswordFile) != "" {
		if _, err := requireClientLocalMaintenancePasswordFile(options.PasswordFile); err != nil {
			return err
		}
	}
	return nil
}

// submitRemoteApplicationMaintenance routes only operations with a qualified
// remote lifecycle owner. Backup, restore and update need a remote archive
// channel that the selected authority does not advertise, so they are refused
// explicitly: routing alone is not proof of support.
func submitRemoteApplicationMaintenance(ctx context.Context, resolved resolvedApplication,
	options applicationMaintenanceOptions, out, errOut io.Writer) error {
	// Routing decision first: capability discovery never depends on a client
	// reference that happens to be spelled badly, so an unsupported intent is
	// refused before any transport or pending write.
	if options.Operation != "destroy" {
		return remoteApplicationMaintenanceUnsupported(resolved, options.Operation)
	}
	// Even remotely, a client may only name its own local safe channels: the
	// authority never resolves a workstation path.
	if err := validateMaintenanceClientChannels(options); err != nil {
		return err
	}
	if !options.Confirmed {
		return applicationMaintenanceApprovalRequired(resolved, options.Operation)
	}
	if _, err := applicationMaintenanceAuthority(resolved); err != nil {
		return err
	}
	return executeRemoteApplicationMaintenanceDestroy(ctx, resolved, options, out, errOut)
}

func remoteApplicationMaintenanceUnsupported(resolved resolvedApplication, operation string) error {
	return machine.NewError(machine.ErrorCapabilityMissing,
		fmt.Sprintf("Remote %s maintenance has no qualified lifecycle owner on the selected authority.", operation),
		"Keep this operation on the installation that owns the deployment state, or qualify the remote maintenance adapter before selecting this Target.",
		false)
}

// executeRemoteApplicationMaintenanceDestroy routes the destructive operation to
// the existing remote destroy authority: the retained publication, its
// consequence plan and its owned cleanup are the same owners a direct teardown
// uses. This client never builds a remote maintenance engine.
func executeRemoteApplicationMaintenanceDestroy(ctx context.Context, resolved resolvedApplication, options applicationMaintenanceOptions, out, errOut io.Writer) error {
	args := append(options.applicationArgs(), "--yes")
	if options.FullReset {
		args = append(args, "--full-reset")
	}
	return maintenanceLifecycleOwner(ctx, options.Operation, resolved, args, out, errOut)
}

func runApplicationMaintenanceOperation(ctx context.Context, resolved resolvedApplication,
	options applicationMaintenanceOptions, out, errOut io.Writer) error {
	args := options.applicationArgs()
	switch options.Operation {
	case "backup":
		if options.PasswordFile == "" {
			return applicationMaintenancePasswordFileRequired()
		}
		archive, err := clientLocalMaintenanceArchive(options.Artifact, resolved, options)
		if err != nil {
			return err
		}
		if err := maintenanceLifecycleOwner(ctx, options.Operation, resolved,
			append(args, "--password-file", options.PasswordFile, "--output", archive), out, errOut); err != nil {
			return err
		}
		return commitApplicationMaintenanceWindow(ctx, resolved, options.Operation, maintenanceFileDigest(archive))
	case "restore":
		if options.PasswordFile == "" {
			return applicationMaintenancePasswordFileRequired()
		}
		archive, err := requireClientLocalMaintenanceArtifact(options.Artifact)
		if err != nil {
			return err
		}
		// The client reads the recovery secret over the explicit safe channel to
		// prove the reference is usable before the archive is opened. The bytes
		// are zeroed here: the lifecycle owner owns the cryptographic use of the
		// secret through its own protected reader.
		secret, err := readMaintenanceSecretInput(options.PasswordFile)
		if err != nil {
			return err
		}
		defer zeroBytes(secret)
		if err := maintenanceLifecycleOwner(ctx, options.Operation, resolved,
			append([]string{archive}, append(args, "--password-file", options.PasswordFile)...), out, errOut); err != nil {
			return err
		}
		return commitApplicationMaintenanceWindow(ctx, resolved, options.Operation, maintenanceFileDigest(archive))
	case "update":
		if applicationUpdateHasDurableState(resolved.Manifest) && options.PasswordFile == "" && !options.NoBackup {
			return applicationMaintenanceRecoveryChoiceRequired(resolved)
		}
		if options.PasswordFile != "" {
			args = append(args, "--backup-password-file", options.PasswordFile)
		}
		if options.NoBackup {
			args = append(args, "--no-backup")
		}
		if options.Confirmed {
			args = append(args, "--yes")
		}
		if err := maintenanceLifecycleOwner(ctx, options.Operation, resolved, args, out, errOut); err != nil {
			return err
		}
		return commitApplicationMaintenanceWindow(ctx, resolved, options.Operation, maintenanceFileDigest(options.PasswordFile))
	case "destroy":
		if !options.Confirmed {
			return applicationMaintenanceApprovalRequired(resolved, options.Operation)
		}
		args = append(args, "--yes")
		if options.FullReset {
			args = append(args, "--full-reset")
		}
		if err := maintenanceLifecycleOwner(ctx, options.Operation, resolved, args, out, errOut); err != nil {
			return err
		}
		return commitApplicationMaintenanceWindow(ctx, resolved, options.Operation, "")
	default:
		return applicationMaintenanceUnknownOperation(options.Operation)
	}
}

// applicationMaintenanceAuthority binds a submission to the installation record
// that actually owns the deployment. Absence is an ownership failure, never
// something a client may route around with a local path.
func applicationMaintenanceAuthority(resolved resolvedApplication) (application.MaintenanceAuthority, error) {
	// The maintenance client never resolves a deployment from a client path and
	// never creates state: the authority record for the selected deployment is the
	// only thing it can submit maintenance against.
	record, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
	if err != nil {
		if isMissingDeploymentAuthority(err) {
			return application.MaintenanceAuthority{}, machine.NewError(machine.ErrorOwnershipAmbiguous,
				"No authority record exists for the selected application deployment.",
				"Run the lifecycle on the installation that owns this deployment before submitting maintenance.", false)
		}
		return application.MaintenanceAuthority{}, err
	}
	persisted, found, err := application.LoadMaintenanceAuthority(resolved.stateRoot())
	if err != nil {
		return application.MaintenanceAuthority{}, err
	}
	own := application.MaintenanceAuthorityRecord{
		ApplicationID: record.Identity.ApplicationID,
		DeploymentID:  record.Identity.DeploymentID,
		Target:        record.Identity.Target,
		Environment:   resolved.Manifest.Environment,
	}
	if found && (persisted.ApplicationID != own.ApplicationID || persisted.DeploymentID != own.DeploymentID) {
		return application.MaintenanceAuthority{}, machine.NewError(machine.ErrorOwnershipAmbiguous,
			"The retained maintenance window belongs to a different deployment.",
			"Inspect the protected deployment state before submitting maintenance.", false)
	}
	resolved2, err := application.ResolveMaintenanceAuthority(persisted, own)
	if err != nil {
		return application.MaintenanceAuthority{}, err
	}
	resolved2.Candidate = application.MaintenanceWindowCandidate(record)
	return resolved2, nil
}

// commitApplicationMaintenanceWindow records one verified maintenance operation
// as the durable window's next revision. The retained record is the only counter:
// a window that exists continues at exactly one past its revision, a deployment
// with no window yet opens its history at revision one. Re-submitting the
// operation the window already records re-sends the retained record itself, so
// the binding the authority wrote cannot mismatch by construction and the
// operation is reported as already recorded instead of replayed.
func commitApplicationMaintenanceWindow(ctx context.Context, resolved resolvedApplication, operation, archive string) error {
	// A window advances only for an operation the lifecycle owner actually
	// converged. A submission that stopped mid-flight records nothing.
	if !maintenanceMutationCompleted(ctx, nil) {
		return maintenanceSubmissionCancelled()
	}
	if err := maintenanceArchiveIsReadable(archive); err != nil {
		return err
	}
	current, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		return err
	}
	record := current.MaintenanceAuthorityRecord
	if current.RecordExists && current.Operation == operation && current.ArchiveBinding == archive {
		// The window already records this exact operation and recovery unit: re-send
		// the retained record itself. Its binding came from the authority that wrote
		// it, so a replay cannot accidentally claim a new revision.
		_, isNew, err := maintenanceWindowOwner(resolved.stateRoot(), current.MaintenanceAuthorityRecord)
		if err != nil {
			return err
		}
		if !isNew {
			return errMaintenanceOperationAlreadyRecorded
		}
		return nil
	}
	record.Operation = operation
	record.ArchiveBinding = archive
	record.Revision = current.OperationRevision()
	record.Digest = application.MaintenanceDigest(current.Target, current.Environment, operation,
		record.Revision, current.Candidate, archive)
	_, isNew, err := maintenanceWindowOwner(resolved.stateRoot(), record)
	if err != nil {
		return err
	}
	if !isNew {
		return errMaintenanceOperationAlreadyRecorded
	}
	return nil
}

var errMaintenanceOperationAlreadyRecorded = errors.New(
	"maintenance window already records this exact durable operation; inspect before submitting again")

// maintenanceArchiveIsReadable rejects a binding the client cannot establish, so
// an unreadable or empty archive can never be recorded as a recovery point.
func maintenanceArchiveIsReadable(binding string) error {
	switch binding {
	case "":
		return nil
	case "sha256:invalid", "sha256:unreadable":
		return machine.NewError(machine.ErrorValidationFailed,
			"The maintenance archive binding could not be established on the client.",
			"Export the recovery unit to the client and re-submit; nothing was recorded.", false)
	default:
		return nil
	}
}

// clientLocalMaintenanceArchive resolves the archive the client will hold. A
// supplied reference is validated; otherwise the client's own default backup
// location is used. Neither case derives a Target-host path.
func clientLocalMaintenanceArchive(explicit string, resolved resolvedApplication, options applicationMaintenanceOptions) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return requireClientLocalMaintenanceArtifact(explicit)
	}
	archive, err := defaultGuidedBackupPath(resolved.Manifest.Name, resolved.Manifest.Environment, options.Now)
	if err != nil {
		return "", err
	}
	return requireClientLocalMaintenanceArtifact(archive)
}

// requireClientLocalMaintenanceArtifact admits only an existing, cleanly spelled
// client file reference. Relative names are workstation guesses, symlinks and
// devices are not recovery units, and Target-host paths are never valid
// maintenance input, so all of them fail closed.
func requireClientLocalMaintenanceArtifact(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", machine.NewError(machine.ErrorValidationFailed,
			"A maintenance archive reference is required.",
			"Pass --artifact with the absolute client-local file reference of the archive you hold.", false)
	}
	if !filepath.IsAbs(path) {
		return "", machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance archive reference %q is a client filename, not a safe channel reference.", path),
			"A laptop filename is never a server-local path; pass the absolute reference of the archive you hold.", false)
	}
	if filepath.Clean(path) != path {
		return "", machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance archive reference %q is not a clean path.", path),
			"Pass the canonical absolute path without . or .. segments.", false)
	}
	if gitWorktreeContaining(path) {
		return "", machine.NewError(machine.ErrorValidationFailed,
			"Maintenance archive reference resolves inside a Git worktree.",
			"Choose a client-local path outside the repository so archived state cannot be committed.", false)
	}
	info, err := maintenanceReferenceStat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", machine.NewError(machine.ErrorValidationFailed,
				"The maintenance archive reference does not exist on the client.",
				"Export or transfer the recovery unit to the client first; the authority never reads a path it was only told about.", false)
		}
		return "", fmt.Errorf("inspect maintenance archive reference: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance archive reference %q is not a regular client file.", path),
			"Reference the recovery unit itself, not a link, device or directory.", false)
	}
	if info.Size() == 0 {
		return "", machine.NewError(machine.ErrorValidationFailed,
			"The maintenance archive reference is empty.",
			"Re-export the recovery unit before submitting it.", false)
	}
	return path, nil
}

// requireClientLocalMaintenancePasswordFile keeps the existing secret channel:
// an owner-only regular client-local file, never a plaintext value or a path the
// authority would have to resolve on the Target.
func requireClientLocalMaintenancePasswordFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance secret input %q is a client filename, not a safe channel reference.", redactMaintenanceInput(path)),
			"Reference an owner-only local password file; a plaintext value is never accepted as secret input.", false)
	}
	clean, err := requireClientLocalMaintenanceArtifact(path)
	if err != nil {
		return "", err
	}
	info, err := maintenanceReferenceStat(clean)
	if err != nil {
		return "", fmt.Errorf("inspect maintenance password file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", machine.NewError(machine.ErrorValidationFailed,
			fmt.Sprintf("Maintenance password file %s is accessible by group or others (%o).", clean, info.Mode().Perm()),
			"Restrict the client-local file to owner-only before submitting maintenance.", false)
	}
	if info.Size() < minMaintenanceSecretBytes {
		return "", applicationMaintenanceSecretInputTooShort()
	}
	return clean, nil
}

// redactMaintenanceInput keeps a rejected plaintext secret out of the error,
// machine result and logs: only a non-reversible digest marker is reported.
func redactMaintenanceInput(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// maintenanceReferenceStat is the client-side existence channel. Production uses
// Lstat so a symlink is visible; tests keep the same behavior.
var maintenanceReferenceStat = func(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func applicationMaintenancePasswordFileRequired() error {
	return machine.NewError(machine.ErrorValidationFailed,
		"Maintenance secret input requires a password-file reference.",
		"Pass --password-file with an owner-only client-local file; plaintext secret values are never accepted.", false)
}

func applicationMaintenanceRecoveryChoiceRequired(resolved resolvedApplication) error {
	return &machine.Error{
		Code:        machine.ErrorApprovalRequired,
		CauseCode:   "recovery_choice_required",
		Message:     "Updating an application with durable managed state requires an explicit recovery choice.",
		Resource:    resolved.Manifest.Name,
		Remediation: "requires operator approval",
		Next:        "Provide --password-file for an encrypted recovery point or set --no-backup to accept the risk explicitly.",
	}
}

func applicationMaintenanceApprovalRequired(resolved resolvedApplication, operation string) error {
	return &machine.Error{
		Code:        machine.ErrorApprovalRequired,
		CauseCode:   "destructive_approval_required",
		Message:     fmt.Sprintf("Maintenance operation %s is destructive and requires explicit approval.", operation),
		Resource:    resolved.Manifest.Name,
		Remediation: "requires operator approval",
		Next:        "Re-submit with --yes after reviewing the concrete consequence plan.",
	}
}

func applicationMaintenanceUnknownOperation(operation string) error {
	return machine.NewError(machine.ErrorValidationFailed,
		fmt.Sprintf("Unknown maintenance operation %q.", operation),
		"Use one of "+strings.Join(maintenanceOperations, ", ")+".", false)
}
