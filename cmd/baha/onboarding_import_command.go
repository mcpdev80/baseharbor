package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/onboardingbundle"
	"github.com/mcpdev80/baseharbor/internal/targetaccess"
)

// runOnboardImport is a single, fail-closed user path. Only a server that
// presents the pinned Core CA, consumes the pre-issued grant and proves its
// installation/tenant identity may create a Target on this workstation.
func runOnboardImport(ctx context.Context, args []string, out io.Writer) error {
	args, trustOptions, err := stripOnboardTrustOptions(args)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(onboardingInput)
	invitationPath, requestedTarget, err := parseOnboardImportArgs(args)
	if err != nil {
		return err
	}
	bundle, err := onboardingbundle.Read(invitationPath, time.Now())
	if err != nil {
		return err
	}
	// Check all predictable local conflicts before consuming the one-use
	// enrollment grant. A rejected or already-configured workstation must not
	// burn an otherwise valid invitation.
	// The workstation Target name is a local alias. Keep the Core's own Target
	// selector separately in the protected connection profile; aliases must
	// never change the authority named by the one-use invitation.
	cfg, _, err := deployment.LoadConfigForMutation()
	if err != nil {
		return err
	}
	targetName := requestedTarget
	if targetName == "" {
		targetName = nextOnboardingTargetName(cfg, organizationEnvironment(ctx), bundle.RuntimeProvider)
		if !noInput(ctx) && readerIsTerminal(onboardingInput) {
			targetName, err = promptLine(reader, out, "Workstation Target name", targetName)
			if err != nil {
				return fmt.Errorf("read workstation Target name: %w", err)
			}
		}
	}
	if err := deployment.ValidateTargetName(targetName); err != nil {
		return err
	}
	if _, exists := cfg.Targets[targetName]; exists {
		return errors.New("remote Core Target already exists; never replace it with an invitation")
	}
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		return err
	}
	profileDir := filepath.Join(dataRoot, "remote-connections", targetName)
	profilePath := filepath.Join(profileDir, "profile.json")
	if _, err := os.Lstat(profilePath); err == nil {
		return errors.New("remote connection profile already exists; refusing replacement")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	identityRoot := filepath.Join(dataRoot, "remote-workstations")
	// Prepare owned local storage before spending the remote one-use grant.
	// Refuse symlinked/public ancestors rather than letting MkdirAll follow
	// a pre-existing path into an attacker-controlled location.
	if err := ensurePrivateOnboardingDirectoryTree(dataRoot); err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(dataRoot, "remote-connections"),
		profileDir,
		identityRoot,
	} {
		if err := ensurePrivateOnboardingDirectory(dir); err != nil {
			return err
		}
	}
	expectedIdentity := filepath.Join(identityRoot, bundle.InstallationID+"-"+bundle.Station)
	if _, err := os.Lstat(expectedIdentity); err == nil {
		return errors.New("workstation identity already exists; refusing replacement")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	device, err := onboardingbundle.Redeem(ctx, bundle, time.Now())
	if err != nil {
		return err
	}
	if err := verifyWorkstationCore(ctx, bundle, device); err != nil {
		return err
	}
	identityPath, err := onboardingbundle.PersistIdentity(identityRoot, bundle, device, time.Now())
	if err != nil {
		return err
	}
	saved := false
	defer func() {
		if !saved {
			_ = os.RemoveAll(identityPath)
		}
	}()
	// A concurrent importer cannot replace an existing profile: the writer
	// below uses an exclusive create after the preflight.
	digest := sha256.Sum256([]byte(bundle.CAPEM))
	profile := map[string]any{
		"version": remoteConnectionProfileVersion,
		"name":    targetName,
		"core": map[string]string{
			"installation_id": bundle.CoreInstallationID,
			"url":             bundle.CoreURL,
		},
		"trust": map[string]string{
			"ca_pem": bundle.CAPEM,
			"sha256": hex.EncodeToString(digest[:]),
		},
		"targets": []map[string]string{{
			"name":             targetName,
			"core_target":      bundle.CoreTarget,
			"tenant_id":        bundle.InstallationID,
			"runtime_provider": bundle.RuntimeProvider,
		}},
		"device": map[string]string{"identity_dir": identityPath},
	}
	payload, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := writeExclusiveOnboardingProfile(profilePath, append(payload, '\n')); err != nil {
		return err
	}
	defer func() {
		if !saved {
			_ = os.Remove(profilePath)
		}
	}()
	accessName := "remote-" + strings.ReplaceAll(bundle.CoreInstallationID, "-", "")[:12]
	_, err = createTargetDefinition(ctx, machineTargetCreateInput{
		Name:            targetName,
		TenantID:        bundle.InstallationID,
		RuntimeProvider: bundle.RuntimeProvider,
		AccessProvider:  string(targetaccess.ProviderCoreRemote),
		Access:          accessName,
		Reference:       bundle.CoreURL,
		Scope:           "dev",
		Default:         cfg.DefaultTarget == "",
	})
	if err != nil {
		return fmt.Errorf("register verified remote Core Target: %w", err)
	}
	saved = true
	if cfg.DefaultTarget == "" {
		// Selection is a convenience, never a replacement for authorization:
		// every later API operation verifies the current device certificate.
		if err := writePersistedTarget(targetName); err != nil {
			return fmt.Errorf("Core Target enrolled, but activate it with baha target activate %s: %w", targetName, err)
		}
	}
	_, err = fmt.Fprintf(out, "Remote Core %s connected. Workstation Target: %s (Core Target: %s)\nUse it with: baha use %s\n", bundle.CoreInstallationID, targetName, bundle.CoreTarget, targetName)
	if err != nil {
		return err
	}
	return offerImportedHostTrust(ctx, reader, out, targetName, []byte(bundle.CAPEM), trustOptions)
}

func parseOnboardImportArgs(args []string) (invitationPath, targetName string, err error) {
	targetProvided := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--target":
			return "", "", usageError("use --target-name for the imported workstation alias", "Run baha onboard --import PATH [--target-name NAME].")
		case arg == "--target-name":
			if targetProvided || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", "", usageError("--target-name requires one Target name", "Run baha onboard --import PATH [--target-name NAME].")
			}
			i++
			targetProvided = true
			targetName = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--target-name="):
			if targetProvided {
				return "", "", usageError("duplicate --target-name", "Pass --target-name once.")
			}
			targetProvided = true
			targetName = strings.TrimSpace(strings.TrimPrefix(arg, "--target-name="))
		default:
			if strings.HasPrefix(arg, "-") || invitationPath != "" {
				return "", "", usageError("baha onboard --import needs one private invitation file", "Run baha onboard --import PATH [--target-name NAME].")
			}
			invitationPath = arg
		}
	}
	if invitationPath == "" {
		return "", "", usageError("baha onboard --import needs one private invitation file", "Run baha onboard --import PATH [--target-name NAME].")
	}
	if targetProvided && targetName == "" {
		return "", "", usageError("--target-name requires one Target name", "Run baha onboard --import PATH [--target-name NAME].")
	}
	return invitationPath, targetName, nil
}

func nextOnboardingTargetName(cfg deployment.Config, environment, runtime string) string {
	environment = strings.ToLower(strings.TrimSpace(environment))
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	if environment == "" {
		environment = "dev"
	}
	prefix := environment + "-" + runtime + "-"
	for sequence := 1; ; sequence++ {
		candidate := prefix + strconv.Itoa(sequence)
		if _, exists := cfg.Targets[candidate]; !exists {
			return candidate
		}
	}
}

func verifyWorkstationCore(ctx context.Context, bundle onboardingbundle.Bundle, identity onboardingbundle.EnrolledIdentity) error {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(bundle.CAPEM)) {
		return errors.New("Core trust is missing")
	}
	pair, err := tls.X509KeyPair(identity.CertificatePEM, identity.PrivateKeyPEM)
	if err != nil {
		return errors.New("verified workstation certificate and private key do not match")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair},
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout: 15 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bundle.CoreURL+"/api/v1/management/installation", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("verify newly enrolled Core workstation identity: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Core refused enrolled device (HTTP %d); Target was not created", resp.StatusCode)
	}
	var got struct {
		Version        string `json:"version"`
		InstallationID string `json:"installation_id"`
		TenantID       string `json:"tenant_id"`
		IdentityIssuer string `json:"identity_issuer"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 16<<10))
	dec.DisallowUnknownFields()
	// The shared installation endpoint also returns operator_issuer, which
	// is intentionally irrelevant to certificate-based device authentication.
	var payload map[string]json.RawMessage
	if err := dec.Decode(&payload); err != nil {
		return errors.New("invalid Core installation response")
	}
	for key := range payload {
		switch key {
		case "version", "installation_id", "tenant_id", "identity_issuer", "operator_issuer":
		default:
			return errors.New("Core installation response uses unknown fields")
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &got) != nil || got.Version != managementInstallationVersion ||
		got.InstallationID != bundle.CoreInstallationID ||
		got.TenantID != bundle.InstallationID || got.IdentityIssuer == "" {
		return errors.New("remote Core installation, tenant or issuer differs from enrollment bundle")
	}
	var rest any
	if err := dec.Decode(&rest); err != io.EOF {
		return errors.New("Core returned trailing installation data")
	}
	return nil
}

func writeExclusiveOnboardingProfile(path string, payload []byte) (err error) {
	// O_EXCL makes a concurrent import and any pre-existing profile fail
	// closed. Unlike rename, it cannot silently replace a valid profile.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func checkPrivateOnboardingDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("onboarding directory must be a private real directory (0700): %s", path)
	}
	return nil
}

func ensurePrivateOnboardingDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create private onboarding directory %s: %w", path, err)
	}
	return checkPrivateOnboardingDirectory(path)
}

func ensurePrivateOnboardingDirectoryTree(path string) error {
	if err := ensureOnboardingDirectoryAncestors(path); err != nil {
		return err
	}
	return checkPrivateOnboardingDirectory(path)
}

func ensureOnboardingDirectoryAncestors(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("onboarding path ancestor must be a real directory: %s", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return fmt.Errorf("cannot create onboarding directory root %s", path)
	}
	if err := ensureOnboardingDirectoryAncestors(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create private onboarding directory ancestor %s: %w", path, err)
	}
	info, err = os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("onboarding path ancestor must be a real directory: %s", path)
	}
	return nil
}
