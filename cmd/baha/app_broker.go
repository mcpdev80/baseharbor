package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
)

func ensureAndStartRuntimeBroker(ctx context.Context, progress io.Writer, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	runtimeImage := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	refreshMutableImage := runtimebroker.IsMutableDevelopmentImage(runtimeImage)
	if refreshMutableImage {
		cli.ReportActivityDetail(progress, "checking current development runtime image")
		if err := compose.PullImage(ctx, runtimeImage); err != nil {
			return fmt.Errorf("refresh development runtime image: %w", err)
		}
	}
	if err := ensureAndStartRuntimeProviderExecutor(ctx, progress, compose, platformFiles, m, refreshMutableImage); err != nil {
		return err
	}
	identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
	workloadDNSNames := []string{}
	runtimeServices := map[string]struct{}{}
	for _, service := range application.RuntimeAuthorizedServices(m) {
		runtimeServices[service] = struct{}{}
		workloadDNSNames = append(workloadDNSNames, service)
	}
	for _, source := range m.Metrics.Sources {
		if _, ok := runtimeServices[source.Service]; ok {
			workloadDNSNames = append(workloadDNSNames, application.MetricsTargetAlias(m, source.Service))
		}
	}
	mtlsFiles, identityChanged, err := openbao.EnsureRuntimeMTLSIdentity(ctx, compose, platformFiles, identity, files, workloadDNSNames)
	if err != nil {
		return fmt.Errorf("converge runtime mTLS identity: %w", err)
	}
	brokerFiles, err := runtimebroker.Ensure(m, files, mtlsFiles)
	if err != nil {
		return fmt.Errorf("materialize application runtime broker: %w", err)
	}
	project := runtimebroker.ProjectName(m)
	if err := compose.ConfigProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("validate application runtime broker: %w", err)
	}
	if identityChanged || refreshMutableImage {
		// Runtime identity files are installed atomically and mutable development
		// images can change behind the same tag. Recreate the broker so readiness
		// always verifies the desired identity rather than a stale container.
		if err := compose.DownProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
			return fmt.Errorf("recreate application runtime broker for desired identity: %w", err)
		}
	}
	if err := compose.UpProjectProgress(ctx, project, brokerFiles.Compose, files.Env, func(detail string) {
		cli.ReportActivityDetail(progress, detail)
	}); err != nil {
		return fmt.Errorf("start application runtime broker: %w", err)
	}
	cli.ReportActivityDetail(progress, "waiting for runtime broker readiness")
	verifyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var verifyErr error
	for verifyCtx.Err() == nil {
		verifyErr = verifyRuntimeBrokerRunning(verifyCtx, compose, m, files)
		if verifyErr == nil {
			cli.ReportActivityDetail(progress, "runtime broker ready")
			return nil
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("application runtime broker readiness failed: %w", verifyErr)
}

func ensureAndStartRuntimeProviderExecutor(ctx context.Context, progress io.Writer, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, refreshMutableImage bool) error {
	if !requiresRuntimeObjectStorageExecutor(m) {
		return nil
	}
	if platformFiles.Compose == "" || platformFiles.Env == "" {
		return errors.New("BaseHarbor control-plane runtime is required for runtime provider executor PKI")
	}
	issuer := openbao.NewServiceIssuer(compose, platformFiles)
	providerFiles, _, adminCredentialsPath, err := objectstorage.EnsureSharedProvider(ctx, compose, issuer)
	if err != nil {
		return fmt.Errorf("converge runtime object-storage provider: %w", err)
	}
	s3Endpoint, err := objectstorage.ServiceContainerEndpoint(providerFiles)
	if err != nil {
		return fmt.Errorf("resolve runtime object-storage HTTPS endpoint: %w", err)
	}
	s3Trust, err := objectstorage.ServiceTrustBundle(providerFiles)
	if err != nil {
		return fmt.Errorf("resolve runtime object-storage trust bundle: %w", err)
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return fmt.Errorf("resolve BaseHarbor data directory for runtime executor: %w", err)
	}
	identityDir := filepath.Join(dataDir, "runtime-executor", "identity")
	identity, identityChanged, err := openbao.EnsureRuntimeExecutorMTLSIdentity(ctx, compose, platformFiles, identityDir)
	if err != nil {
		return fmt.Errorf("converge runtime executor mTLS identity: %w", err)
	}
	executorFiles, err := runtimeexecutor.EnsureFiles(dataDir, identity, adminCredentialsPath, s3Endpoint, s3Trust)
	if err != nil {
		return fmt.Errorf("materialize runtime provider executor: %w", err)
	}
	if err := compose.ConfigProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env); err != nil {
		return fmt.Errorf("validate runtime provider executor: %w", err)
	}
	if identityChanged || refreshMutableImage {
		if err := compose.DownProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env); err != nil {
			return fmt.Errorf("recreate runtime provider executor for desired identity: %w", err)
		}
	}
	if err := compose.UpProjectProgress(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env, func(detail string) {
		cli.ReportActivityDetail(progress, detail)
	}); err != nil {
		return fmt.Errorf("start runtime provider executor: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimeexecutor.ProjectName, executorFiles.Compose, executorFiles.Env)
	if err != nil {
		return fmt.Errorf("inspect runtime provider executor: %w", err)
	}
	if len(services) != 1 || services[0] != runtimeexecutor.ServiceName {
		return errors.New("runtime provider executor is not running")
	}
	return nil
}

func waitRuntimeBrokerReady(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for {
		lastErr = verifyRuntimeBrokerRunning(waitCtx, compose, m, files)
		if lastErr == nil {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("application runtime broker did not become ready within %s: %w", timeout, lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func verifyRuntimeBrokerRunning(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	project := runtimebroker.ProjectName(m)
	services, err := compose.RunningServicesProject(ctx, project, brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("inspect application runtime broker: %w", err)
	}
	if len(services) != 1 || services[0] != runtimebroker.ServiceName {
		return errors.New("application runtime broker is not running")
	}
	out, err := compose.ExecProject(ctx, project, brokerFiles.Compose, files.Env, runtimebroker.ServiceName,
		"curl", "--fail", "--silent", "--show-error",
		"--resolve", "baseharbor-runtime:8443:127.0.0.1",
		"--cacert", "/run/baseharbor/identity/ca.pem",
		"--cert", "/run/secrets/probe-client-cert",
		"--key", "/run/secrets/probe-client-key",
		runtimebroker.RuntimeURL+"/readyz",
	)
	if err != nil {
		return fmt.Errorf("application runtime broker mTLS readiness probe failed: %w", err)
	}
	var ready struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal([]byte(out), &ready); err != nil || ready.Status != "ready" {
		return errors.New("application runtime broker readiness response is invalid")
	}
	if err := verifyRuntimeBrokerBuildIdentity(ready.Version, ready.Commit); err != nil {
		return err
	}
	if strings.TrimSpace(brokerFiles.DocsURL) != "" {
		if err := verifyRuntimeBrokerDocs(ctx, brokerFiles.DocsURL, files); err != nil {
			return err
		}
	}
	return nil
}

func verifyRuntimeBrokerBuildIdentity(actualVersion, actualCommit string) error {
	expectedVersion := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	expectedCommit := strings.TrimSpace(commit)
	actualVersion = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(actualVersion), "v"))
	actualCommit = strings.TrimSpace(actualCommit)
	if actualVersion == "" {
		return errors.New("runtime broker image is incompatible: build identity is missing")
	}
	if expectedVersion != "" && actualVersion != expectedVersion {
		developmentPair := expectedVersion == "dev" && actualVersion == "edge"
		if !developmentPair {
			return fmt.Errorf("runtime broker image is incompatible: CLI version %s requires runtime version %s, got %s", expectedVersion, expectedVersion, actualVersion)
		}
	}
	if expectedCommit != "" && expectedCommit != "none" && actualCommit != expectedCommit {
		if actualCommit == "" {
			actualCommit = "unknown"
		}
		return fmt.Errorf("runtime broker image is incompatible: CLI commit %s, runtime commit %s", expectedCommit, actualCommit)
	}
	return nil
}

func verifyRuntimeBrokerDocs(ctx context.Context, docsURL string, files application.RuntimeFiles) error {
	caPEM, err := os.ReadFile(filepath.Join(files.Bindings, "runtime-identity", "ca.pem"))
	if err != nil {
		return fmt.Errorf("read Runtime Docs CA certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return errors.New("Runtime Docs CA certificate is invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docsURL, nil)
	if err != nil {
		return fmt.Errorf("build Runtime Docs readiness request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Runtime Docs HTTPS readiness failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("Runtime Docs HTTPS readiness returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func printRuntimeBrokerDocs(out io.Writer, files application.RuntimeFiles) {
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil || strings.TrimSpace(brokerFiles.DocsURL) == "" {
		return
	}
	fmt.Fprintf(out, "[INFO] runtime-broker    Swagger/OpenAPI: %s\n", brokerFiles.DocsURL)
}

func stopRuntimeBroker(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	if err := compose.DownProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("stop application runtime broker: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimebroker.ProjectName(m), brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("verify application runtime broker stop: %w", err)
	}
	if len(services) != 0 {
		return errors.New("verify application runtime broker stop: broker is still running")
	}
	return nil
}
