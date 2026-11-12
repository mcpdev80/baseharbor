package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func devCommand() *cli.Command {
	return &cli.Command{
		Name:    "dev",
		Summary: "Manage local development conveniences",
		Usage:   "baha dev <credentials|domain>",
		Children: []*cli.Command{
			{
				Name:    "credentials",
				Summary: "Show or rotate the target-scoped development management login",
				Usage:   "baha dev credentials [--reset] [--username USER] [--password-file FILE]",
				Long:    "Shows the explicit local development management login for the effective Target. The secret is revealed only by this command and is never included in status, plan, doctor, evidence or application manifests. --reset generates a new strong password. --password-file installs an explicit password from an owner-only file.",
				Run:     devCredentialsCommand,
			},
			{
				Name:    "domain",
				Summary: "Show or configure the target-scoped development domain",
				Usage:   "baha dev domain [DOMAIN]",
				Long:    "Shows the development domain used to derive canonical local application and management URLs. Supplying DOMAIN updates the target-wide value; application manifests are not modified.",
				Run:     devDomainCommand,
			},
		},
	}
}

func devDomainCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "dev domain")
	if err != nil {
		return err
	}
	args = filtered
	if len(args) > 1 {
		return usageError("baha dev domain accepts at most one domain", "Run 'baha dev domain --help' for usage.")
	}
	result, err := configureDevelopmentDomain(ctx, firstArgument(args))
	if err != nil {
		return err
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	targetName, domain := result.Target, result.Domain
	fmt.Fprintf(out, "Development domain\n")
	fmt.Fprintf(out, "  Target  %s\n", targetName)
	fmt.Fprintf(out, "  Domain  %s\n", domain)
	fmt.Fprintln(out, "  Scope   target / dev")
	if len(args) == 1 {
		fmt.Fprintln(out, "Run 'baha up' to reconcile canonical development routes.")
	}
	return nil
}

func devCredentialsCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "dev credentials")
	if err != nil {
		return err
	}
	args = filtered
	reset := false
	username := ""
	passwordFile := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reset":
			reset = true
		case "--username", "--password-file":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(args[i]+" requires a value", "Run 'baha dev credentials --help' for usage.")
			}
			value := strings.TrimSpace(args[i+1])
			i++
			if args[i-1] == "--username" {
				username = value
			} else {
				passwordFile = value
			}
		default:
			return unknownOptionUsage("baha dev credentials", args[i], "--reset", "--username", "--password-file")
		}
	}

	credentials, targetName, err := configureDevelopmentCredentials(ctx, reset, username, passwordFile)
	if err != nil {
		return err
	}
	if format == outputJSON {
		path, err := devaccess.Path(targetName, "dev")
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"target": targetName, "environment": "dev", "username": credentials.Username, "protected_file": path})
	}
	fmt.Fprintf(out, "Developer management access\n")
	fmt.Fprintf(out, "  Target    %s\n", targetName)
	fmt.Fprintf(out, "  Scope     dev\n")
	fmt.Fprintf(out, "  Username  %s\n", credentials.Username)
	fmt.Fprintf(out, "  Password  %s\n", credentials.Password)
	fmt.Fprintln(out, "\nThis explicit command reveals the secret. Normal BaseHarbor status, plans and evidence never do.")
	if reset || passwordFile != "" || username != "" {
		fmt.Fprintln(out, "Run 'baha up' to reconcile the updated login into selected development management UIs.")
	}
	return nil
}

func readDevAccessPasswordFile(path string) (string, error) {
	value, err := readProtectedSecretInput(path)
	if err != nil {
		return "", err
	}
	defer zeroBytes(value)
	if len(value) > 64<<10 {
		return "", errors.New("developer access password file exceeds the 65536-byte limit")
	}
	password := strings.TrimSuffix(string(value), "\n")
	password = strings.TrimSuffix(password, "\r")
	if strings.TrimSpace(password) == "" || strings.ContainsAny(password, "\r\n") {
		return "", errors.New("developer access password file must contain exactly one nonempty line")
	}
	return password, nil
}

// A bootstrap fallback is permitted only for an explicitly local Target with
// neither a persisted Core installation nor existing runtime files. A remote
// selection or any ambiguous/malformed state must fail closed.
func localCredentialBootstrapAllowed(ctx context.Context) (bool, error) {
	selected, err := effectiveTarget(ctx)
	if err != nil {
		return false, err
	}
	if selected.AccessProvider != "" && selected.AccessProvider != "local" {
		return false, nil
	}
	root, err := targetRuntimeStateRoot(selected)
	if err != nil {
		return false, err
	}
	if _, err = coreinstallation.Load(root); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if _, err = existingTargetRuntimeFiles(ctx); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func reconcileDeveloperCredentialAuthority(ctx context.Context, target, environment string, candidate devaccess.Credentials, replace bool) (devaccess.Credentials, error) {
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		// Runtime resolution can fail for reasons other than initial bootstrap
		// (missing target, inaccessible runtime, failed provider discovery).
		// None of these errors proves that there is no managed authority.
		// Never silently promote a local candidate to authoritative material.
		if allowed, bootstrapErr := localCredentialBootstrapAllowed(ctx); bootstrapErr == nil && allowed {
			return candidate, nil
		}
		return devaccess.Credentials{}, fmt.Errorf("resolve managed credential authority: %w", err)
	}
	state, err := openBaoInspect(ctx, compose, files)
	if err != nil {
		return devaccess.Credentials{}, fmt.Errorf("inspect managed credential authority: %w", err)
	}
	if !state.Initialized {
		return candidate, nil
	}
	if state.Sealed {
		return devaccess.Credentials{}, errors.New("managed credential authority is sealed; developer credentials were not changed")
	}
	if err := openBaoCheckManager(ctx, compose, files); err != nil {
		return devaccess.Credentials{}, errors.New("managed credential authority is unavailable; developer credentials were not changed")
	}

	sum := sha256.Sum256([]byte(strings.TrimSpace(target)))
	ref := "targets/" + hex.EncodeToString(sum[:12]) + "/human/dev-management"
	encode := func(credentials devaccess.Credentials) ([]byte, error) {
		return json.Marshal(struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}{Username: credentials.Username, Password: credentials.Password})
	}
	decode := func(data []byte) (devaccess.Credentials, error) {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(data, &payload); err != nil || strings.TrimSpace(payload.Username) == "" || payload.Password == "" {
			return devaccess.Credentials{}, errors.New("managed developer credential is invalid")
		}
		return devaccess.Credentials{Username: payload.Username, Password: payload.Password}, nil
	}

	if !replace {
		data, readErr := managedCredentialSeam.get(ctx, compose, files, ref)
		switch {
		case readErr == nil:
			return decode(data)
		case errors.Is(readErr, platformopenbao.ErrManagedCredentialNotFound):
			// First reconciliation imports the existing protected bootstrap
			// value, after which OpenBao is authoritative.
			// Device rebind: a fresh device that never reconciled has no managed
			// object yet. Recover the opt-in local credential and seed the managed
			// authority exactly once; a local store that is absent fails closed
			// instead of prompting for or accepting new material.
			result, rebindingErr := recoverLocalCredential(ctx, target)
			switch {
			case rebindingErr != nil:
				return devaccess.Credentials{}, fmt.Errorf("rebind developer credential authority: %w", rebindingErr)
			case result == devaccess.Unrecovered:
				// No opt-in local credential: fail closed, no prompt, no material.
				return devaccess.Credentials{}, devaccess.ErrNoRecoverableCredential
			default:
				// The local opt-in credential seeded the managed authority exactly
				// once (one Set on this path). The managed object is now authoritative,
				// so read it back and return the actually-managed value instead of
				// issuing a second SetManagedCredential(candidate). A managed object
				// that appeared after the initial not-found lookup keeps its value:
				// we Get, never overwrite it with the stale local candidate.
				managed, managedErr := managedCredentialSeam.get(ctx, compose, files, ref)
				if managedErr != nil {
					return devaccess.Credentials{}, fmt.Errorf("read back managed credential after seed: %w", managedErr)
				}
				return decode(managed)
			}
		default:
			return devaccess.Credentials{}, readErr
		}
	}
	data, err := encode(candidate)
	if err != nil {
		return devaccess.Credentials{}, errors.New("encode developer credential for managed storage")
	}
	if err := managedCredentialSeam.set(ctx, compose, files, ref, data); err != nil {
		return devaccess.Credentials{}, err
	}
	return candidate, nil
}

// devRecoveryFactory builds the device-rebind recovery bound to the existing
// OpenBao managed authority. It is a package seam so tests can inject a recovery
// that counts the managed Get/Set pairs on the reconcile path; production wires
// the existing openBaoManagedAuthorityChecker/openBaoManagedCredentialStore.
var devRecoveryFactory = func() devaccess.Recovery {
	return devaccess.Recovery{
		Managed: openBaoManagedAuthorityChecker{},
		Store:   openBaoManagedCredentialStore{},
	}
}

// recoverLocalCredential performs the device-rebind recovery for a target using
// the opt-in local developer credential authority. A fresh device that never
// reconciled has no managed object yet; the local credential seeds the managed
// authority exactly once (one Set on this path) and the caller returns the
// actually-managed value instead of issuing a second write. When neither the
// local store nor a managed object is present the rebind fails closed
// (Unrecovered, no prompt, no material).
func recoverLocalCredential(ctx context.Context, target string) (devaccess.RecoveryResult, error) {
	recovery := devRecoveryFactory()
	return recovery.Recover(ctx, target)
}

// seedManagedCredentialIfAbsent seeds the fresh-device managed credential with
// the create-only / compare-and-swap variant of the existing OpenBao managed
// authority. It returns ErrManagedCredentialAlreadyExists when a concurrent
// value already created the object, so the caller returns the managed winner
// instead of clobbering it. It is the seed half of one Set per path; the
// replace/rotate path keeps the unconditional SetManagedCredential write.
func seedManagedCredentialIfAbsent(ctx context.Context, executor openbao.Executor, files bhruntime.Files, ref string, data []byte) error {
	return platformopenbao.SetManagedCredentialIfAbsent(ctx, executor, files, ref, data)
}

// managedCredentialSeamFuncs is the managed-get/set boundary the dev-access
// reconcile path depends on. managedCredentialSeam defaults to the production
// OpenBao authority and is overridable in tests to count the managed Get/Set
// pairs exercised by one reconcile. Exactly one production path is wired here;
// there is no second store.
type managedCredentialSeamFuncs struct {
	get func(ctx context.Context, executor openbao.Executor, files bhruntime.Files, reference string) ([]byte, error)
	set func(ctx context.Context, executor openbao.Executor, files bhruntime.Files, reference string, value []byte) error
}

// managedCredentialSeam is the single managed-get/set boundary the dev-access
// reconcile path depends on.
var managedCredentialSeam = managedCredentialSeamFuncs{
	get: func(ctx context.Context, executor openbao.Executor, files bhruntime.Files, reference string) ([]byte, error) {
		return platformopenbao.GetManagedCredential(ctx, executor, files, reference)
	},
	set: func(ctx context.Context, executor openbao.Executor, files bhruntime.Files, reference string, value []byte) error {
		return platformopenbao.SetManagedCredential(ctx, executor, files, reference, value)
	},
}

// managedCredentialSeamDefaults is the production value of managedCredentialSeam.
var managedCredentialSeamDefaults = managedCredentialSeam

// defaultOpenBaoRuntime is the production value of the openBaoRuntime seam.
var defaultOpenBaoRuntime = openBaoRuntime

// defaultDevRecoveryFactory is the production value of devRecoveryFactory.
var defaultDevRecoveryFactory = devRecoveryFactory

// openBaoInspect and openBaoCheckManager are the OpenBao runtime-boundary seams
// the dev-access reconcile path depends on. They default to the production
// platformopenbao authority and are overridable in tests to reach the managed
// seam without a real runtime.
var openBaoInspect = func(ctx context.Context, executor openbao.Executor, files bhruntime.Files) (platformopenbao.State, error) {
	return platformopenbao.Inspect(ctx, executor, files)
}

var openBaoCheckManager = func(ctx context.Context, executor openbao.Executor, files bhruntime.Files) error {
	return platformopenbao.CheckManager(ctx, executor, files)
}

// openBaoManagedAuthorityChecker reports whether a managed-secret object already
// exists for a target. It wires the existing OpenBao authority, never a new one.
type openBaoManagedAuthorityChecker struct{}

func (openBaoManagedAuthorityChecker) Exists(ctx context.Context, target string) (bool, error) {
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		// Before the managed secret provider exists there is no managed object.
		return false, nil
	}
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return false, fmt.Errorf("inspect managed credential authority: %w", err)
	}
	if !state.Initialized || state.Sealed {
		return false, nil
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(target)))
	ref := "targets/" + hex.EncodeToString(sum[:12]) + "/human/dev-management"
	_, readErr := managedCredentialSeam.get(ctx, compose, files, ref)
	switch {
	case readErr == nil:
		return true, nil
	case errors.Is(readErr, platformopenbao.ErrManagedCredentialNotFound):
		return false, nil
	default:
		return false, fmt.Errorf("inspect managed credential for rebind: %w", readErr)
	}
}

// openBaoManagedCredentialStore seeds the managed authority once from the local
// opt-in credential using a create-only/CAS write (SetManagedCredentialIfAbsent)
// over the existing OpenBao managed mount, never a new secret store. If a
// concurrent managed value already appeared the seed is a no-op and the existing
// managed value wins.
type openBaoManagedCredentialStore struct{}

func (openBaoManagedCredentialStore) Persist(ctx context.Context, target, username, password string) error {
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		return fmt.Errorf("seed managed credential authority: %w", err)
	}
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return fmt.Errorf("inspect managed credential authority: %w", err)
	}
	if !state.Initialized || state.Sealed {
		return errors.New("managed credential authority is not available; developer credential was not changed")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(target)))
	ref := "targets/" + hex.EncodeToString(sum[:12]) + "/human/dev-management"
	data, err := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password})
	if err != nil {
		return fmt.Errorf("encode developer credential for managed storage: %w", err)
	}
	if seedErr := seedManagedCredentialIfAbsent(ctx, compose, files, ref, data); seedErr != nil {
		return fmt.Errorf("seed managed credential authority: %w", seedErr)
	}
	return nil
}

// recoverMissingLocalDeveloperCredential consults the managed authority BEFORE
// creating a new workstation projection. A missing local projection does not
// imply that a previously enrolled target needs new credentials.
func recoverMissingLocalDeveloperCredential(ctx context.Context, target, environment string) (devaccess.Credentials, bool, error) {
	_, err := devaccess.Load(target, environment)
	if err == nil {
		return devaccess.Credentials{}, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return devaccess.Credentials{}, false, err
	}
	authoritative, err := reconcileDeveloperCredentialAuthority(ctx, target, environment, devaccess.Credentials{}, false)
	if err != nil {
		return devaccess.Credentials{}, false, err
	}
	if authoritative.Username == "" && authoritative.Password == "" {
		// Explicitly uninitialized OpenBao: normal local bootstrap can proceed.
		return devaccess.Credentials{}, false, nil
	}
	if authoritative.Username == "" || authoritative.Password == "" {
		return devaccess.Credentials{}, false, errors.New("managed developer credential is incomplete")
	}
	projected, err := devaccess.Configure(target, environment, authoritative.Username, authoritative.Password)
	if err != nil {
		return devaccess.Credentials{}, false, fmt.Errorf("project recovered developer credential: %w", err)
	}
	return projected, true, nil
}

func ensureAuthoritativeDeveloperCredentials(ctx context.Context, target, environment string) (devaccess.Credentials, error) {
	// Enabling a development management UI opts into creating its first shared
	// login. Recovery alone must still reject absent credentials; this explicit
	// provisioning path may generate them when neither authority has a value.
	if recovered, ok, err := recoverMissingLocalDeveloperCredential(ctx, target, environment); err != nil && !errors.Is(err, devaccess.ErrNoRecoverableCredential) {
		return devaccess.Credentials{}, err
	} else if ok {
		return recovered, nil
	}
	candidate, err := devaccess.Ensure(target, environment)
	if err != nil {
		return devaccess.Credentials{}, err
	}
	authoritative, err := reconcileDeveloperCredentialAuthority(ctx, target, environment, candidate, false)
	if err != nil {
		return devaccess.Credentials{}, err
	}
	if authoritative != candidate {
		return devaccess.Configure(target, environment, authoritative.Username, authoritative.Password)
	}
	return candidate, nil
}
