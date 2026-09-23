package main

import (
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

type applicationTLSObservation struct {
	Mode            string `json:"mode"`
	Hostname        string `json:"hostname,omitempty"`
	Healthy         bool   `json:"healthy"`
	Certificate     string `json:"certificate,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	SourceState     string `json:"source_state,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
	Detail          string `json:"detail,omitempty"`
}

func collectApplicationTLSObservation(resolved resolvedApplication) (*applicationTLSStatus, *applicationTLSObservation, error) {
	if !resolved.FromRepository {
		return nil, nil, nil
	}
	state, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
	if err != nil {
		return nil, &applicationTLSObservation{Healthy: false, Detail: "TLS deployment state could not be read"}, err
	}
	if strings.TrimSpace(state.TLSMode) == "" {
		return nil, nil, nil
	}
	status, inspectErr := inspectApplicationTLS(resolved)
	if inspectErr != nil {
		return nil, &applicationTLSObservation{
			Mode:     state.TLSMode,
			Hostname: state.Hostname,
			Healthy:  false,
			Detail:   conciseTLSStatusError(inspectErr),
		}, inspectErr
	}
	observation := applicationTLSObservationFromStatus(status)
	return &status, &observation, nil
}

func applicationTLSObservationFromStatus(status applicationTLSStatus) applicationTLSObservation {
	observation := applicationTLSObservation{
		Mode:            status.State.TLSMode,
		Hostname:        status.State.Hostname,
		Healthy:         true,
		UpdateAvailable: status.UpdateAvailable,
	}
	switch status.State.TLSMode {
	case "existing":
		if status.Installed != nil {
			observation.Certificate = certificateDisplayName(status.Installed)
			observation.ExpiresAt = status.Installed.NotAfter.UTC().Format(time.RFC3339)
		}
		switch {
		case status.Source == nil:
			observation.SourceState = "unavailable"
			observation.Detail = "configured certificate source unavailable"
		case status.UpdateAvailable:
			observation.SourceState = "update-available"
			observation.Detail = "different source certificate available"
		default:
			observation.SourceState = "verified"
			observation.Detail = "installed certificate matches configured source"
		}
	case "acme":
		observation.SourceState = "delegated"
		observation.Detail = "ACME lifecycle delegated to workload TLS provider"
	case "local":
		observation.SourceState = "local"
		observation.Detail = "local development TLS"
	default:
		observation.Healthy = false
		observation.SourceState = "unknown"
		observation.Detail = "unknown TLS mode"
	}
	return observation
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
		return fmt.Errorf("certificate update preflight: runtime orchestration is unavailable: %w", err)
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
		rollbackManagedExposure(ctx, preparedExposure)
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
	cmd.Long += " Repository deployment TLS mode and certificate state are rendered as part of the same status view."
	cmd.Run = func(ctx context.Context, args []string, out, errOut io.Writer) error {
		filtered, format, err := parseReadOutputArgs(args, "app status")
		if err != nil {
			return err
		}
		result, err := collectApplicationStatusResult(ctx, store, filtered)
		if err != nil {
			return err
		}

		if format == outputJSON {
			if err := writeJSON(out, result); err != nil {
				return err
			}
		} else {
			renderApplicationStatusWithExtra(ctx, out, errOut, result.StatusResult, func(term *cli.Terminal) {
				if result.tlsStatus == nil && result.tlsErr == nil {
					return
				}
				term.Section("TLS")
				if result.tlsErr != nil {
					term.Result("FAILED", "certificate", conciseTLSStatusError(result.tlsErr))
					return
				}
				renderApplicationTLSStatus(term, *result.tlsStatus)
			})
			if resolved, resolveErr := resolveApplication(store, filtered, "status"); resolveErr == nil {
				if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
					printRuntimeBrokerDocs(out, files)
				}
			}
		}

		if result.State == "stopped" || result.State == "not_applied" {
			return nil
		}
		if !result.Ready {
			return cli.Presented(errors.New("application is not ready"))
		}
		return nil
	}
	return cmd
}

func renderApplicationTLSStatus(term *cli.Terminal, status applicationTLSStatus) {
	switch status.State.TLSMode {
	case "existing":
		term.Result("READY", "certificate", certificateDisplayName(status.Installed)+" · expires "+formatCertificateTime(status.Installed.NotAfter))
		if status.Source == nil {
			if status.Warning != "" {
				term.Result("WARN", "certificate-source", "configured source unavailable")
				if term.Verbose() {
					term.Diagnostic("TLS source detail: %s\n", status.Warning)
				}
			}
			return
		}
		if status.UpdateAvailable {
			term.Result("UPDATED", "certificate-source", "different source certificate available")
		} else {
			term.Result("VERIFIED", "certificate-source", "installed certificate matches configured source")
		}
	case "acme":
		term.Result("READY", "certificate", "ACME lifecycle delegated to workload TLS provider")
	case "local":
		term.Result("READY", "certificate", "local development TLS")
	default:
		term.Result("WARN", "certificate", "unknown TLS mode")
	}
}

func conciseTLSStatusError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "expired"):
		return "certificate expired"
	case strings.Contains(text, "does not cover"):
		return "certificate does not cover configured hostname"
	case strings.Contains(text, "mismatch"):
		return "certificate and private key do not match"
	default:
		return "certificate state could not be verified"
	}
}

func appDoctorRepairCommandWithTLS(store application.Store) *cli.Command {
	return appDoctorRepairCommand(store)
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
	state, err := loadRepositoryInitStateFromStateRoot(resolved.stateRoot())
	if err != nil {
		return applicationTLSStatus{}, err
	}
	status := applicationTLSStatus{State: state}
	if strings.TrimSpace(state.TLSMode) == "" {
		return status, nil
	}
	if state.TLSMode == "existing" {
		if state.TLSDir == "" {
			state.TLSDir = filepath.Join(resolved.stateRoot(), repositoryTLSDirName)
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
