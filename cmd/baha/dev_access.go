package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
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

func reconcileDeveloperCredentialAuthority(ctx context.Context, target, environment string, candidate devaccess.Credentials, replace bool) (devaccess.Credentials, error) {
	compose, files, err := openBaoRuntime(ctx)
	if err != nil {
		// Before the managed secret provider exists, the owner-only bootstrap
		// projection is the temporary authority. Once OpenBao is initialized,
		// this path is imported and OpenBao becomes authoritative.
		return candidate, nil
	}
	state, err := platformopenbao.Inspect(ctx, compose, files)
	if err != nil {
		return devaccess.Credentials{}, fmt.Errorf("inspect managed credential authority: %w", err)
	}
	if !state.Initialized {
		return candidate, nil
	}
	if state.Sealed {
		return devaccess.Credentials{}, errors.New("managed credential authority is sealed; developer credentials were not changed")
	}
	if err := platformopenbao.CheckManager(ctx, compose, files); err != nil {
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
		data, readErr := platformopenbao.GetManagedCredential(ctx, compose, files, ref)
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
			if result, rebindingErr := recoverLocalCredential(ctx, target); rebindingErr != nil {
				return devaccess.Credentials{}, fmt.Errorf("rebind developer credential authority: %w", rebindingErr)
			} else if result == devaccess.Unrecovered {
				return devaccess.Credentials{}, devaccess.ErrNoRecoverableCredential
			}
		default:
			return devaccess.Credentials{}, readErr
		}
	}
	data, err := encode(candidate)
	if err != nil {
		return devaccess.Credentials{}, errors.New("encode developer credential for managed storage")
	}
	if err := platformopenbao.SetManagedCredential(ctx, compose, files, ref, data); err != nil {
		return devaccess.Credentials{}, err
	}
	return candidate, nil
}

// recoverLocalCredential performs the device-rebind recovery for a target using
// the opt-in local developer credential authority. A fresh device that never
// reconciled has no managed object yet; the local credential seeds the managed
// authority exactly once. When neither the local store nor a managed object is
// present the rebind fails closed (Unrecovered, no prompt, no material).
func recoverLocalCredential(ctx context.Context, target string) (devaccess.RecoveryResult, error) {
	recovery := devaccess.Recovery{
		Managed: openBaoManagedAuthorityChecker{},
		Store:   openBaoManagedCredentialStore{},
	}
	return recovery.Recover(ctx, target)
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
	_, readErr := platformopenbao.GetManagedCredential(ctx, compose, files, ref)
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
// opt-in credential. It wires the existing SetManagedCredential path, never a
// new secret store.
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
	if err := platformopenbao.SetManagedCredential(ctx, compose, files, ref, data); err != nil {
		return fmt.Errorf("seed managed credential authority: %w", err)
	}
	return nil
}

func ensureAuthoritativeDeveloperCredentials(ctx context.Context, target, environment string) (devaccess.Credentials, error) {
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
