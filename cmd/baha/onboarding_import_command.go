package main

import (
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
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return usageError("baha onboard --import needs exactly one private invitation file", "Run baha onboard --import PATH.")
	}
	bundle, err := onboardingbundle.Read(args[0], time.Now())
	if err != nil {
		return err
	}
	// Check all predictable local conflicts before consuming the one-use
	// enrollment grant. A rejected or already-configured workstation must not
	// burn an otherwise valid invitation.
	targetName := "core-" + strings.ReplaceAll(bundle.CoreInstallationID, "-", "")[:12]
	cfg, _, err := deployment.LoadConfigForMutation()
	if err != nil {
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
	for _, dir := range []string{profileDir, identityRoot} {
		if err := checkPrivateOnboardingDirectory(dir); err != nil {
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
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return err
	}
	// A second collision check handles another CLI process that raced the
	// preflight. The profile writer below must also use O_EXCL.
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
	_, err = fmt.Fprintf(out, "Remote Core %s connected. Target: %s\n", bundle.CoreInstallationID, targetName)
	return err
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
