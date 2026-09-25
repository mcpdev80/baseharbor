package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func reloadConfig(ctx context.Context, client *http.Client, endpoint string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/-/reload", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Prometheus reload returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func waitReady(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/-/ready", nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func prometheusTargetDiagnostic(ctx context.Context, client *http.Client, endpoint string, app application.Manifest, source string) string {
	target := strings.TrimRight(endpoint, "/") + "/api/v1/targets?state=active"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ActiveTargets []struct {
				Labels    map[string]string `json:"labels"`
				Health    string            `json:"health"`
				LastError string            `json:"lastError"`
			} `json:"activeTargets"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil || payload.Status != "success" {
		return ""
	}
	for _, active := range payload.Data.ActiveTargets {
		if active.Labels["baseharbor_application"] != app.Name ||
			active.Labels["baseharbor_environment"] != app.Environment ||
			active.Labels["baseharbor_source"] != source {
			continue
		}
		if strings.TrimSpace(active.LastError) != "" {
			return fmt.Sprintf("target health=%s last_error=%s", active.Health, active.LastError)
		}
		return fmt.Sprintf("target health=%s but no up=1 sample was observed", active.Health)
	}
	return "Prometheus has no active target matching the application metrics binding"
}

func VerifyProviderSources(ctx context.Context, m application.Manifest) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	return VerifyProviderSourcesAt(ctx, m, dataDir, "")
}

func VerifyProviderSourcesAt(ctx context.Context, m application.Manifest, dataDir, namespace string) error {
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return err
	}
	placement, err := application.ResolveProviderPlacement(m, capability.ProviderPrometheus)
	if err != nil {
		return err
	}
	allowedApplications := []string{m.Name}
	if placement.Scope == capability.ScopeShared {
		if files, fileErr := ExistingProviderFilesAt(dataDir, namespace, m); fileErr == nil {
			if registrations, regErr := readRegistrations(files.Registrations); regErr == nil {
				allowedApplications = allowedApplications[:0]
				for _, registration := range registrations {
					allowedApplications = append(allowedApplications, registration.Application)
				}
			}
		}
	}
	sources, err := observability.ListMetrics(
		placement,
		allowedApplications,
		policy.Collect[application.MetricsSourceApplicationProvider],
		policy.Collect[application.MetricsSourcePlatformProvider],
	)
	if err != nil || len(sources) == 0 {
		return err
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client, err := providerHTTPClient(m, files)
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for _, source := range sources {
		query := fmt.Sprintf(
			`up{job="baseharbor-providers",baseharbor_provider=%q,baseharbor_source=%q}`,
			string(source.Provider), source.ID,
		)
		ticker := time.NewTicker(time.Second)
		var last error
		for {
			ok, err := queryUp(deadline, client, endpoint, query)
			if err == nil && ok {
				ticker.Stop()
				break
			}
			if err != nil {
				last = err
			} else {
				last = errors.New("provider target has not produced an up=1 sample yet")
			}
			select {
			case <-deadline.Done():
				ticker.Stop()
				return fmt.Errorf("verify provider metrics %s: %w", source.ID, last)
			case <-ticker.C:
			}
		}
	}
	return nil
}

func prometheusAccessEnvironment(m application.Manifest, registrations []sourceRegistration) string {
	managed := !isDevelopmentEnvironment(m.Environment)
	for _, registration := range registrations {
		if !isDevelopmentEnvironment(registration.Environment) {
			managed = true
			break
		}
	}
	if managed {
		return "prod"
	}
	return "dev"
}

func isDevelopmentEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "dev", "development":
		return true
	default:
		return false
	}
}

func prometheusAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "prometheus-access",
		Upstream:         "http://prometheus:9090",
		PublishedPortEnv: "BASEHARBOR_PROMETHEUS_PORT",
		ContainerPort:    8443,
		Networks:         []string{"access", "publish"},
		RequireClient:    true,
	}
}

func providerHTTPClient(m application.Manifest, files ProviderFiles) (*http.Client, error) {
	policy, err := serviceaccess.Resolve(m.Environment, "prometheus", serviceaccess.AuthenticationMTLS)
	if err != nil {
		return nil, err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		return nil, fmt.Errorf("load Prometheus service access identity: %w", err)
	}
	return serviceaccess.NewHTTPClientForPolicy(material, policy)
}

func queryUp(ctx context.Context, client *http.Client, endpoint, query string) (bool, error) {
	values := url.Values{"query": []string{query}}
	target := strings.TrimRight(endpoint, "/") + "/api/v1/query?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return false, fmt.Errorf("Prometheus query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	if payload.Status != "success" {
		return false, errors.New("Prometheus query did not succeed")
	}
	for _, result := range payload.Data.Result {
		if len(result.Value) != 2 {
			continue
		}
		if value, ok := result.Value[1].(string); ok && value == "1" {
			return true, nil
		}
	}
	return false, nil
}

func allocatePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
