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
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/runtimeexecutor"
)

func runtimeComponentDataRoot(files application.RuntimeFiles) (string, error) {
	if strings.TrimSpace(files.Namespace) != "" {
		return deployment.TargetStateRoot(files.Namespace)
	}
	return bhruntime.DataDir("")
}

var errRuntimeBrokerIncompatible = errors.New("runtime broker image is incompatible")

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
	if err := ensureAndStartRuntimeProviderExecutor(ctx, progress, compose, platformFiles, m, files, refreshMutableImage); err != nil {
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
	issuer := openbao.NewServiceIssuer(compose, platformFiles)
	mtlsFiles, identityChanged, err := openbao.EnsureRuntimeMTLSIdentity(ctx, issuer, identity, files, workloadDNSNames)
	if err != nil {
		return fmt.Errorf("converge runtime mTLS identity: %w", err)
	}
	otlpBinding, hasOTLPBinding, err := application.ExistingRuntimeOTLPBinding(m, files)
	if err != nil {
		return fmt.Errorf("resolve runtime broker OTLP binding: %w", err)
	}
	traceTarget := ""
	traceSecurity := observability.Security{}
	if hasOTLPBinding {
		traceTarget = otlpBinding.ContainerEndpoint
		traceSecurity = observability.Security{
			TLSRequired:       strings.HasPrefix(strings.ToLower(otlpBinding.ContainerEndpoint), "https://"),
			TrustFile:         otlpBinding.CAFile,
			ClientCertificate: otlpBinding.ClientCertFile,
			ClientKey:         otlpBinding.ClientKeyFile,
			ServerName:        "otel-collector-access",
		}
		if otlpBinding.ClientCertFile != "" && otlpBinding.ClientKeyFile != "" {
			traceSecurity.Authentication = "mtls"
		}
	}
	brokerProject := runtimebroker.ProjectNameForRuntime(m, files)
	if err := application.ReconcileRuntimeComponentObservability(m, application.RuntimeComponentObservability{
		ID:               "runtime-broker:" + brokerProject,
		Provider:         capability.ProviderRuntimeBroker,
		Class:            observability.SourceApplicationProvider,
		Scope:            capability.ScopeApplication,
		OwnerApplication: m.Name,
		MetricsNetwork:   runtimebroker.ObservabilityNetworkNameForRuntime(m, files),
		MetricsTarget:    "baseharbor-runtime:8443",
		MetricsPath:      "/metrics",
		MetricsSecurity: observability.Security{
			TLSRequired:       true,
			Authentication:    "mtls",
			TrustFile:         mtlsFiles.CA,
			ClientCertificate: mtlsFiles.ClientCert,
			ClientKey:         mtlsFiles.ClientKey,
			ServerName:        "baseharbor-runtime",
		},
		LogsTarget:     observability.RuntimeTarget(brokerProject, runtimebroker.ServiceName),
		TracesTarget:   traceTarget,
		TracesSecurity: traceSecurity,
	}); err != nil {
		return fmt.Errorf("register runtime broker observability: %w", err)
	}
	brokerFiles, err := runtimebroker.Ensure(m, files, mtlsFiles)
	if err != nil {
		return fmt.Errorf("materialize application runtime broker: %w", err)
	}
	project := brokerProject
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
		if errors.Is(verifyErr, errRuntimeBrokerIncompatible) {
			return verifyErr
		}
		select {
		case <-verifyCtx.Done():
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("application runtime broker readiness failed: %w", verifyErr)
}

func ensureAndStartRuntimeProviderExecutor(ctx context.Context, progress io.Writer, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles, refreshMutableImage bool) error {
	if !requiresRuntimeObjectStorageExecutor(m) {
		return nil
	}
	if platformFiles.Compose == "" || platformFiles.Env == "" {
		return errors.New("BaseHarbor control-plane runtime is required for runtime provider executor PKI")
	}
	issuer := openbao.NewServiceIssuer(compose, platformFiles)
	dataDir, err := runtimeComponentDataRoot(files)
	if err != nil {
		return fmt.Errorf("resolve target data directory for runtime executor: %w", err)
	}
	providerFiles, _, adminCredentialsPath, err := objectstorage.ExistingReadySharedProviderAt(ctx, dataDir, files.Namespace)
	if err != nil {
		providerFiles, _, adminCredentialsPath, err = objectstorage.EnsureSharedProviderAt(ctx, compose, issuer, dataDir, files.Namespace)
		if err != nil {
			return fmt.Errorf("converge runtime object-storage provider: %w", err)
		}
	}
	s3Endpoint, err := objectstorage.ServiceContainerEndpoint(providerFiles)
	if err != nil {
		return fmt.Errorf("resolve runtime object-storage HTTPS endpoint: %w", err)
	}
	s3Trust, err := objectstorage.ServiceTrustBundle(providerFiles)
	if err != nil {
		return fmt.Errorf("resolve runtime object-storage trust bundle: %w", err)
	}
	identityDir := filepath.Join(dataDir, "runtime-executor", "identity")
	identity, identityChanged, err := openbao.EnsureRuntimeExecutorMTLSIdentity(ctx, issuer, identityDir)
	if err != nil {
		return fmt.Errorf("converge runtime executor mTLS identity: %w", err)
	}
	var observabilityBinding []runtimeexecutor.ObservabilityBinding
	if binding, found, bindErr := application.ExistingRuntimeOTLPBinding(m, files); bindErr != nil {
		return fmt.Errorf("resolve runtime executor OTLP binding: %w", bindErr)
	} else if found && binding.Provider == capability.ProviderOTelCollector {
		observabilityBinding = append(observabilityBinding, runtimeexecutor.ObservabilityBinding{
			Endpoint:   binding.ContainerEndpoint,
			CA:         identity.CA,
			ClientCert: identity.ClientCert,
			ClientKey:  identity.ClientKey,
		})
	}
	traceTarget := ""
	traceSecurity := observability.Security{}
	if len(observabilityBinding) > 0 {
		traceTarget = observabilityBinding[0].Endpoint
		traceSecurity = observability.Security{
			TLSRequired:       true,
			Authentication:    "mtls",
			TrustFile:         observabilityBinding[0].CA,
			ClientCertificate: observabilityBinding[0].ClientCert,
			ClientKey:         observabilityBinding[0].ClientKey,
			ServerName:        "otel-collector-access",
		}
	}
	executorFiles, err := runtimeexecutor.EnsureFilesAt(dataDir, files.Namespace, identity, adminCredentialsPath, s3Endpoint, s3Trust, observabilityBinding...)
	if err != nil {
		return fmt.Errorf("materialize runtime provider executor: %w", err)
	}
	if err := application.ReconcileRuntimeComponentObservability(m, application.RuntimeComponentObservability{
		ID:             "runtime-executor:" + executorFiles.Project,
		Provider:       capability.ProviderRuntimeExecutor,
		Class:          observability.SourcePlatformProvider,
		Scope:          capability.ScopeShared,
		MetricsNetwork: executorFiles.ControlNetwork,
		MetricsTarget:  "baseharbor-runtime-executor:9443",
		MetricsPath:    "/metrics",
		MetricsSecurity: observability.Security{
			TLSRequired:       true,
			Authentication:    "mtls",
			TrustFile:         identity.CA,
			ClientCertificate: identity.ClientCert,
			ClientKey:         identity.ClientKey,
			ServerName:        openbao.RuntimeExecutorDNSName,
		},
		LogsTarget:     observability.RuntimeTarget(executorFiles.Project, runtimeexecutor.ServiceName),
		TracesTarget:   traceTarget,
		TracesSecurity: traceSecurity,
	}); err != nil {
		return fmt.Errorf("register runtime executor observability: %w", err)
	}
	if err := compose.ConfigProject(ctx, executorFiles.Project, executorFiles.Compose, executorFiles.Env); err != nil {
		return fmt.Errorf("validate runtime provider executor: %w", err)
	}
	if identityChanged || refreshMutableImage {
		if err := compose.DownProject(ctx, executorFiles.Project, executorFiles.Compose, executorFiles.Env); err != nil {
			return fmt.Errorf("recreate runtime provider executor for desired identity: %w", err)
		}
	}
	if err := compose.UpProjectProgress(ctx, executorFiles.Project, executorFiles.Compose, executorFiles.Env, func(detail string) {
		cli.ReportActivityDetail(progress, detail)
	}); err != nil {
		return fmt.Errorf("start runtime provider executor: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, executorFiles.Project, executorFiles.Compose, executorFiles.Env)
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
	project := runtimebroker.ProjectNameForRuntime(m, files)
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
		return fmt.Errorf("%w: build identity is missing", errRuntimeBrokerIncompatible)
	}
	if expectedVersion != "" && actualVersion != expectedVersion {
		developmentPair := expectedVersion == "dev" && actualVersion == "edge"
		if !developmentPair {
			return fmt.Errorf("%w: CLI version %s requires runtime version %s, got %s", errRuntimeBrokerIncompatible, expectedVersion, expectedVersion, actualVersion)
		}
	}
	// Commit SHAs are build provenance, not a runtime compatibility contract.
	// Patch-only/docs-only CLI rebuilds may legitimately differ from the
	// published runtime image while still speaking the same versioned API.
	_ = expectedCommit
	_ = actualCommit
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


func destroyRuntimeBroker(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	project := runtimebroker.ProjectNameForRuntime(m, files)
	if err := compose.DestroyProject(ctx, project, brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("destroy application runtime broker: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, project, brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("verify application runtime broker destruction: %w", err)
	}
	if len(services) != 0 {
		return errors.New("verify application runtime broker destruction: broker is still running")
	}
	return nil
}

func stopRuntimeBroker(ctx context.Context, compose bhruntime.Compose, m application.Manifest, files application.RuntimeFiles) error {
	if !application.RequiresRuntimeBroker(m) {
		return nil
	}
	brokerFiles, err := runtimebroker.Existing(files)
	if err != nil {
		return fmt.Errorf("application runtime broker state is missing: %w", err)
	}
	if err := compose.DownProject(ctx, runtimebroker.ProjectNameForRuntime(m, files), brokerFiles.Compose, files.Env); err != nil {
		return fmt.Errorf("stop application runtime broker: %w", err)
	}
	services, err := compose.RunningServicesProject(ctx, runtimebroker.ProjectNameForRuntime(m, files), brokerFiles.Compose, files.Env)
	if err != nil {
		return fmt.Errorf("verify application runtime broker stop: %w", err)
	}
	if len(services) != 0 {
		return errors.New("verify application runtime broker stop: broker is still running")
	}
	return nil
}
