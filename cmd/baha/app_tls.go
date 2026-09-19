package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type applicationTLSStatus struct {
	State           repositoryInitState
	Installed       *x509.Certificate
	Source          *x509.Certificate
	SourcePair      detectedCertificatePair
	UpdateAvailable bool
	Warning         string
}

func appTLSCommand(store application.Store) *cli.Command {
	cmd := &cli.Command{
		Name:    "tls",
		Summary: "Inspect and maintain application TLS certificates",
		Usage:   "baha app tls <command>",
		Long:    "Manages deployment TLS state without exposing private keys to the application. Existing-certificate deployments can inspect a newer certificate in the configured source directory and safely import it into protected BaseHarbor state.",
	}
	cmd.Children = []*cli.Command{
		{
			Name:    "update",
			Summary: "Check for or install a newer existing TLS certificate",
			Usage:   "baha app tls update [--check]",
			Long:    "For TLS mode 'existing', validates the certificate/key pair in the configured source directory, compares it with the installed certificate and refuses certificate downgrades. Without --check, a newer certificate is copied into protected BaseHarbor state and the repository workload is restarted so the new certificate is actually served.",
			Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
				checkOnly := false
				for _, arg := range args {
					switch arg {
					case "--check":
						checkOnly = true
					default:
						return usageError("unknown app tls update option "+arg, "Run 'baha app tls update --help' for usage.")
					}
				}
				resolved, err := resolveApplication(store, nil, "tls update")
				if err != nil {
					return err
				}
				if !resolved.FromRepository {
					return errors.New("application TLS lifecycle requires a repository-owned baseharbor.yaml")
				}
				status, err := inspectApplicationTLS(resolved)
				if err != nil {
					return err
				}
				if status.State.TLSMode == "" {
					return errors.New("application TLS runtime initialization is missing; run 'baha app init'")
				}
				printApplicationTLSStatus(out, status)
				if status.State.TLSMode != "existing" {
					fmt.Fprintf(out, "TLS mode %s is not updated from an external certificate directory.\n", status.State.TLSMode)
					return nil
				}
				if status.Source == nil {
					return fmt.Errorf("certificate source is not ready: %s", status.Warning)
				}
				if !status.UpdateAvailable {
					fmt.Fprintln(out, "Certificate is up to date. No changes were made.")
					return nil
				}
				if status.Installed != nil && !status.Source.NotAfter.After(status.Installed.NotAfter) {
					return fmt.Errorf("refusing TLS certificate downgrade: source expires %s, installed certificate expires %s", formatCertificateTime(status.Source.NotAfter), formatCertificateTime(status.Installed.NotAfter))
				}
				if checkOnly {
					fmt.Fprintln(out, "Certificate update is available. No changes were made.")
					return nil
				}
				return installApplicationTLSUpdate(ctx, out, resolved, status)
			},
		},
	}
	return cmd
}

func installApplicationTLSUpdate(ctx context.Context, out io.Writer, resolved resolvedApplication, status applicationTLSStatus) error {
	certPath := filepath.Join(status.State.TLSDir, "cert.pem")
	keyPath := filepath.Join(status.State.TLSDir, "key.pem")
	oldCert, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read installed certificate before update: %w", err)
	}
	oldKey, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read installed private key before update: %w", err)
	}

	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return fmt.Errorf("certificate update preflight: application runtime state is unavailable: %w", err)
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return fmt.Errorf("certificate update preflight: Compose is unavailable: %w", err)
	}
	preparedExposure, err := prepareManagedExposure(ctx, compose, resolved)
	if err != nil {
		return fmt.Errorf("certificate update preflight: managed exposure is not ready for rotation: %w", err)
	}

	if err := writeNormalizedTLSFiles(status.State.TLSDir, status.SourcePair); err != nil {
		return err
	}
	rollback := func() {
		_ = os.WriteFile(certPath, oldCert, 0o600)
		_ = os.WriteFile(keyPath, oldKey, 0o600)
		_ = os.Chmod(certPath, 0o600)
		_ = os.Chmod(keyPath, 0o600)
	}
	recoverPrevious := func() {
		rollback()
		_, _ = applyRepositoryWorkload(ctx, io.Discard, compose, resolved, files)
		if restoredExposure, prepareErr := prepareManagedExposure(ctx, compose, resolved); prepareErr == nil {
			_ = convergeManagedExposure(ctx, io.Discard, restoredExposure)
		}
	}

	if err := stopManagedExposure(ctx, compose, resolved.Manifest, files); err != nil {
		recoverPrevious()
		return fmt.Errorf("certificate update rolled back because managed exposure could not be stopped: %w", err)
	}
	stopped, err := stopRepositoryWorkload(ctx, compose, resolved, files)
	if err != nil {
		recoverPrevious()
		return fmt.Errorf("certificate update rolled back because the application workload could not be stopped: %w", err)
	}
	if stopped {
		if _, err := applyRepositoryWorkload(ctx, out, compose, resolved, files); err != nil {
			recoverPrevious()
			return fmt.Errorf("certificate update rolled back because the application workload did not recover: %w", err)
		}
	}
	if err := convergeManagedExposure(ctx, out, preparedExposure); err != nil {
		recoverPrevious()
		return fmt.Errorf("certificate update rolled back because managed exposure did not recover: %w", err)
	}

	fmt.Fprintf(out, "[OK] tls-update        installed certificate valid until %s\n", formatCertificateTime(status.Source.NotAfter))
	if stopped {
		fmt.Fprintln(out, "[OK] tls-reload        repository workload restarted and readiness verified")
	} else {
		fmt.Fprintln(out, "[OK] tls-reload        no repository workload restart was required")
	}
	if preparedExposure != nil {
		fmt.Fprintln(out, "[OK] exposure-reload   managed HTTP/TLS exposure reconciled and verified")
	}
	fmt.Fprintln(out, "TLS certificate update completed.")
	return nil
}

func appStatusCommandWithTLS(store application.Store) *cli.Command {
	cmd := appStatusCommand(store)
	baseRun := cmd.Run
	cmd.Long += " Repository deployment TLS mode, certificate expiry and available existing-certificate updates are reported when runtime initialization state is present."
	cmd.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		var base bytes.Buffer
		baseErr := baseRun(ctx, args, &base, errOut)
		_, _ = io.Copy(out, &base)
		resolved, resolveErr := resolveApplication(store, args, "status")
		if resolveErr == nil && resolved.FromRepository {
			if tlsStatus, tlsErr := inspectApplicationTLS(resolved); tlsErr != nil {
				fmt.Fprintln(out)
				fmt.Fprintf(out, "[FAIL] tls               %v\n", tlsErr)
				if baseErr == nil {
					return tlsErr
				}
			} else if tlsStatus.State.TLSMode != "" {
				fmt.Fprintln(out)
				printApplicationTLSStatus(out, tlsStatus)
			}
		}
		return baseErr
	}
	return cmd
}

func appDoctorRepairCommandWithTLS(store application.Store) *cli.Command {
	cmd := appDoctorRepairCommand(store)
	baseRun := cmd.Run
	cmd.Long += " Repository TLS state is also checked for certificate/key validity, FQDN coverage, remaining validity and an available newer certificate in the configured source directory."
	cmd.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		var base bytes.Buffer
		baseErr := baseRun(ctx, args, &base, errOut)
		_, _ = io.Copy(out, &base)
		resolved, resolveErr := resolveApplication(store, doctorApplicationArgs(args), "doctor")
		if resolveErr == nil && resolved.FromRepository {
			if tlsStatus, tlsErr := inspectApplicationTLS(resolved); tlsErr != nil {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "TLS")
				fmt.Fprintf(out, "[FAIL] tls certificate lifecycle: %v\n", tlsErr)
				if baseErr == nil {
					return tlsErr
				}
			} else if tlsStatus.State.TLSMode != "" {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "TLS")
				printApplicationTLSDiagnostics(out, tlsStatus)
			}
		}
		return baseErr
	}
	return cmd
}

func doctorApplicationArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--fix" {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func inspectApplicationTLS(resolved resolvedApplication) (applicationTLSStatus, error) {
	repoRoot := filepath.Dir(resolved.ManifestPath)
	state, err := loadRepositoryInitState(repoRoot)
	if err != nil {
		return applicationTLSStatus{}, err
	}
	status := applicationTLSStatus{State: state}
	if strings.TrimSpace(state.TLSMode) == "" {
		return status, nil
	}
	if state.TLSMode == "existing" {
		if state.TLSDir == "" {
			state.TLSDir = filepath.Join(repoRoot, ".baseharbor", repositoryTLSDirName)
			status.State.TLSDir = state.TLSDir
		}
		installed, err := readManagedCertificate(state.TLSDir, state.Hostname)
		if err != nil {
			return applicationTLSStatus{}, fmt.Errorf("installed TLS certificate: %w", err)
		}
		status.Installed = installed
		if strings.TrimSpace(state.CertDir) == "" {
			status.Warning = "existing TLS mode has no configured certificate source directory"
			return status, nil
		}
		pair, err := detectCertificatePair(state.CertDir, state.Hostname)
		if err != nil {
			status.Warning = "certificate source is unavailable or invalid: " + err.Error()
			return status, nil
		}
		source, err := certificateFromPEM(pair.CertPEM, state.Hostname)
		if err != nil {
			status.Warning = "certificate source is invalid: " + err.Error()
			return status, nil
		}
		status.SourcePair = pair
		status.Source = source
		status.UpdateAvailable = certificateFingerprint(installed) != certificateFingerprint(source)
		if status.UpdateAvailable && !source.NotAfter.After(installed.NotAfter) {
			status.Warning = "source certificate differs but is not newer than the installed certificate"
		}
	}
	return status, nil
}

func readManagedCertificate(tlsDir, hostname string) (*x509.Certificate, error) {
	certPath := filepath.Join(tlsDir, "cert.pem")
	keyPath := filepath.Join(tlsDir, "key.pem")
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	certPEM, leaf, err := normalizeCertificateInput(certData)
	if err != nil {
		return nil, err
	}
	if _, err := tls.X509KeyPair(certPEM, keyData); err != nil {
		return nil, fmt.Errorf("certificate/private-key mismatch: %w", err)
	}
	if err := validateLeafCertificate(leaf, hostname); err != nil {
		return nil, err
	}
	return leaf, nil
}

func certificateFromPEM(data []byte, hostname string) (*x509.Certificate, error) {
	_, leaf, err := normalizeCertificateInput(data)
	if err != nil {
		return nil, err
	}
	if err := validateLeafCertificate(leaf, hostname); err != nil {
		return nil, err
	}
	return leaf, nil
}

func validateLeafCertificate(leaf *x509.Certificate, hostname string) error {
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("certificate is not valid before %s", formatCertificateTime(leaf.NotBefore))
	}
	if !now.Before(leaf.NotAfter) {
		return fmt.Errorf("certificate expired at %s", formatCertificateTime(leaf.NotAfter))
	}
	if hostname != "" && hostname != "localhost" {
		if err := leaf.VerifyHostname(hostname); err != nil {
			return fmt.Errorf("certificate does not cover %s: %w", hostname, err)
		}
	}
	return nil
}

func certificateFingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return fmt.Sprintf("%x", sum[:])
}

func certificateDisplayName(cert *x509.Certificate) string {
	if cert == nil {
		return "unknown"
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	return "unnamed certificate"
}

func formatCertificateTime(value time.Time) string {
	return value.Local().Format("2006-01-02 15:04 MST")
}

func certificateRemaining(value time.Time) string {
	remaining := time.Until(value)
	if remaining <= 0 {
		return "expired"
	}
	days := int(remaining.Hours() / 24)
	if days == 0 {
		return "less than 1 day"
	}
	return fmt.Sprintf("%d days", days)
}

func certificateHealthPrefix(value time.Time) string {
	remaining := time.Until(value)
	switch {
	case remaining <= 0:
		return "[FAIL]"
	case remaining <= 7*24*time.Hour:
		return "[CRITICAL]"
	case remaining <= 30*24*time.Hour:
		return "[WARN]"
	default:
		return "[OK]"
	}
}

func printApplicationTLSStatus(out io.Writer, status applicationTLSStatus) {
	fmt.Fprintf(out, "TLS mode: %s\n", status.State.TLSMode)
	fmt.Fprintf(out, "TLS FQDN: %s\n", status.State.Hostname)
	switch status.State.TLSMode {
	case "existing":
		fmt.Fprintf(out, "%s tls               %s; expires %s (%s remaining)\n", certificateHealthPrefix(status.Installed.NotAfter), certificateDisplayName(status.Installed), formatCertificateTime(status.Installed.NotAfter), certificateRemaining(status.Installed.NotAfter))
		fmt.Fprintf(out, "TLS source: %s\n", status.State.CertDir)
		if status.Source == nil {
			fmt.Fprintf(out, "[WARN] tls-source        %s\n", status.Warning)
			return
		}
		if status.UpdateAvailable {
			fmt.Fprintf(out, "[INFO] tls-update        available: %s; source expires %s\n", certificateDisplayName(status.Source), formatCertificateTime(status.Source.NotAfter))
			fmt.Fprintln(out, "       action: baha app tls update --check")
		} else {
			fmt.Fprintln(out, "[OK] tls-update        installed certificate matches configured source")
		}
		if status.Warning != "" {
			fmt.Fprintf(out, "[WARN] tls-source        %s\n", status.Warning)
		}
	case "acme":
		fmt.Fprintln(out, "[OK] tls               ACME certificate lifecycle is delegated to the workload TLS provider")
	case "local":
		fmt.Fprintln(out, "[OK] tls               local development TLS mode")
	default:
		fmt.Fprintf(out, "[WARN] tls              unknown TLS mode %q\n", status.State.TLSMode)
	}
}

func printApplicationTLSDiagnostics(out io.Writer, status applicationTLSStatus) {
	fmt.Fprintf(out, "[OK] TLS mode          %s\n", status.State.TLSMode)
	fmt.Fprintf(out, "[OK] TLS FQDN          %s\n", status.State.Hostname)
	if status.State.TLSMode != "existing" {
		fmt.Fprintln(out, "[OK] TLS lifecycle     no external certificate source requires BaseHarbor import")
		return
	}
	fmt.Fprintln(out, "[OK] TLS key pair      certificate and private key match")
	fmt.Fprintf(out, "[OK] TLS hostname      certificate covers %s\n", status.State.Hostname)
	fmt.Fprintf(out, "%s TLS expiry        %s remaining; expires %s\n", certificateHealthPrefix(status.Installed.NotAfter), certificateRemaining(status.Installed.NotAfter), formatCertificateTime(status.Installed.NotAfter))
	if status.Source == nil {
		fmt.Fprintf(out, "[WARN] TLS source       %s\n", status.Warning)
		return
	}
	if status.UpdateAvailable {
		fmt.Fprintln(out, "[INFO] TLS update       newer/different source certificate detected; run 'baha app tls update --check'")
	} else {
		fmt.Fprintln(out, "[OK] TLS update        installed certificate matches configured source")
	}
	if status.Warning != "" {
		fmt.Fprintf(out, "[WARN] TLS source       %s\n", status.Warning)
	}
}
