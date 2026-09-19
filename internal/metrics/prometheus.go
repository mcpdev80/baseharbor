package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	ProviderProject = "baseharbor-metrics"
	ProviderService = "prometheus"
	ProviderNetwork = "baseharbor-metrics"
	ProviderImage   = "prom/prometheus:v3.14.0"
)

type Placement struct {
	Scope   capability.ProviderScope
	Project string
	Network string
	Volume  string
	Dir     string
}

func PlacementFor(m application.Manifest) (Placement, error) {
	policy, err := application.MetricsPolicy(m)
	if err != nil {
		return Placement{}, err
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	switch policy.ProviderScope {
	case capability.ScopeShared:
		return Placement{
			Scope: capability.ScopeShared,
			Project: ProviderProject,
			Network: ProviderNetwork,
			Volume: "baseharbor-prometheus-data",
			Dir: filepath.Join(dataDir, "providers", "prometheus", "shared"),
		}, nil
	case capability.ScopeApplication:
		suffix := m.Name + "-" + m.Environment
		return Placement{
			Scope: capability.ScopeApplication,
			Project: "baseharbor-metrics-" + suffix,
			Network: application.MetricsProviderNetworkName(m, capability.ScopeApplication),
			Volume: "baseharbor-prometheus-data-" + suffix,
			Dir: filepath.Join(dataDir, "providers", "prometheus", "applications", m.Name, m.Environment),
		}, nil
	case capability.ScopeExternal:
		return Placement{Scope: capability.ScopeExternal}, nil
	default:
		return Placement{}, fmt.Errorf("unsupported metrics provider scope %q", policy.ProviderScope)
	}
}

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type ProviderFiles struct {
	Dir        string
	Compose    string
	Env        string
	Config     string
	TargetsDir string
}

type Driver struct {
	runtime Runtime
	app     application.Manifest
	client  *http.Client
}

func NewDriver(runtime Runtime, app application.Manifest) *Driver {
	return &Driver{
		runtime: runtime,
		app:     app,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Driver) Descriptor() capability.Provider { return capability.Prometheus }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Metrics {
		return fmt.Errorf("Prometheus provider cannot satisfy %s", resource.Kind)
	}
	if binding.Metrics == nil {
		return errors.New("metrics binding is required")
	}
	value := binding.Metrics
	if value.Direction != "provide" || value.Format != "openmetrics" {
		return errors.New("Prometheus provider requires provide/openmetrics metrics binding")
	}
	if value.Service == "" || value.Port < 1 || value.Port > 65535 || !strings.HasPrefix(value.Path, "/") {
		return errors.New("Prometheus metrics binding is incomplete")
	}
	policy, err := application.MetricsPolicy(d.app)
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return errors.New("metrics collection is disabled by deployment policy")
	}
	if policy.ProviderScope == capability.ScopeExternal {
		return errors.New("external metrics provider requires an external collection adapter")
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	placement, err := PlacementFor(d.app)
	if err != nil {
		return err
	}
	files, err := EnsureProviderFiles(d.app)
	if err != nil {
		return err
	}
	if err := d.runtime.ConfigProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate Prometheus provider configuration: %w", err)
	}
	if err := d.runtime.UpProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start Prometheus provider: %w", err)
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	return waitReady(ctx, d.client, endpoint)
}

func (d *Driver) Bind(_ context.Context, resource capability.Resource, binding capability.Binding) error {
	if binding.Metrics == nil {
		return errors.New("metrics binding is required")
	}
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	target := targetGroup{
		Targets: []string{net.JoinHostPort(application.MetricsTargetAlias(d.app, binding.Metrics.Service), strconv.Itoa(binding.Metrics.Port))},
		Labels: map[string]string{
			"job":                     "baseharbor-applications",
			"baseharbor_application":  d.app.Name,
			"baseharbor_environment":  d.app.Environment,
			"baseharbor_service":      binding.Metrics.Service,
			"baseharbor_source":       resource.Name,
			"baseharbor_metrics_path": binding.Metrics.Path,
		},
	}
	data, err := json.MarshalIndent([]targetGroup{target}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(files.TargetsDir, targetFileName(d.app, resource.Name))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write Prometheus target: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace Prometheus target: %w", err)
	}
	return nil
}

func (d *Driver) Verify(ctx context.Context, resource capability.Resource, _ capability.Binding) error {
	files, err := ExistingProviderFiles(d.app)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		`up{job="baseharbor-applications",baseharbor_application=%q,baseharbor_environment=%q,baseharbor_source=%q}`,
		d.app.Name, d.app.Environment, resource.Name,
	)
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		ok, err := queryUp(deadline, d.client, endpoint, query)
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("Prometheus target has not produced an up=1 sample yet")
		}
		select {
		case <-deadline.Done():
			if last == nil {
				last = deadline.Err()
			}
			return fmt.Errorf("verify Prometheus scrape for %s/%s: %w", d.app.Name, resource.Name, last)
		case <-ticker.C:
		}
	}
}

func PruneApplicationTargets(m application.Manifest, desired map[string]struct{}) error {
	files, err := ExistingProviderFiles(d.app)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(files.TargetsDir)
	if err != nil {
		return err
	}
	prefix := targetFilePrefix(m)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if _, keep := desired[entry.Name()]; keep {
			continue
		}
		if err := os.Remove(filepath.Join(files.TargetsDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale Prometheus target %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func DesiredTargetFiles(m application.Manifest) map[string]struct{} {
	result := make(map[string]struct{}, len(m.Metrics.Sources))
	for _, source := range m.Metrics.Sources {
		result[targetFileName(m, source.Name)] = struct{}{}
	}
	return result
}

func EnsureProviderFiles(m application.Manifest) (ProviderFiles, error) {
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, errors.New("external metrics provider has no BaseHarbor-owned provider files")
	}
	dir := placement.Dir
	targetsDir := filepath.Join(dir, "targets")
	if err := os.MkdirAll(targetsDir, 0o755); err != nil {
		return ProviderFiles{}, fmt.Errorf("create Prometheus provider state: %w", err)
	}
	files := ProviderFiles{
		Dir:        dir,
		Compose:    filepath.Join(dir, "compose.yaml"),
		Env:        filepath.Join(dir, "runtime.env"),
		Config:     filepath.Join(dir, "prometheus.yml"),
		TargetsDir: targetsDir,
	}
	port := ""
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
				port = strings.TrimSpace(value)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ProviderFiles{}, err
	}
	if port == "" {
		value, err := allocatePort()
		if err != nil {
			return ProviderFiles{}, err
		}
		port = strconv.Itoa(value)
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_PROMETHEUS_PORT="+port+"\n"), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Config, []byte(prometheusConfig()), 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.Chmod(files.Config, 0o644); err != nil {
		return ProviderFiles{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(providerComposeYAML(placement)), 0o600); err != nil {
		return ProviderFiles{}, err
	}
	return files, nil
}

func ExistingProviderFiles(m application.Manifest) (ProviderFiles, error) {
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope == capability.ScopeExternal {
		return ProviderFiles{}, os.ErrNotExist
	}
	dir := placement.Dir
	files := ProviderFiles{
		Dir:        dir,
		Compose:    filepath.Join(dir, "compose.yaml"),
		Env:        filepath.Join(dir, "runtime.env"),
		Config:     filepath.Join(dir, "prometheus.yml"),
		TargetsDir: filepath.Join(dir, "targets"),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	placement, err := PlacementFor(m)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return nil
	}
	files, err := ExistingProviderFiles(m)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, placement.Project, files.Compose, files.Env); err != nil {
		return err
	}
	return os.RemoveAll(files.Dir)
}

func SharedProbeManifest() application.Manifest {
	return application.Manifest{Name: "shared-probe", Environment: "dev"}
}

func ExistingSharedProviderFiles() (ProviderFiles, error) {
	m := SharedProbeManifest()
	placement, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, err
	}
	if placement.Scope != capability.ScopeShared {
		return ProviderFiles{}, os.ErrNotExist
	}
	dir := placement.Dir
	files := ProviderFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"),
		Config: filepath.Join(dir, "prometheus.yml"), TargetsDir: filepath.Join(dir, "targets"),
	}
	for _, path := range []string{files.Compose, files.Env, files.Config, files.TargetsDir} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, err
		}
	}
	return files, nil
}

func DestroySharedProvider(ctx context.Context, runtime Runtime) error {
	m := SharedProbeManifest()
	return DestroyProvider(ctx, runtime, m)
}

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Prometheus port")
			}
			return "http://127.0.0.1:" + strconv.Itoa(port), nil
		}
	}
	return "", errors.New("Prometheus port is not materialized")
}

type targetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func targetFilePrefix(m application.Manifest) string {
	return m.Name + "--" + m.Environment + "--"
}

func targetFileName(m application.Manifest, source string) string {
	return targetFilePrefix(m) + source + ".json"
}

func providerComposeYAML(placement Placement) string {
	return fmt.Sprintf(`services:
  prometheus:
    image: prom/prometheus:v3.14.0
    restart: unless-stopped
    user: "65534:65534"
    read_only: true
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.path=/prometheus
    ports:
      - "127.0.0.1:${BASEHARBOR_PROMETHEUS_PORT}:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./targets:/etc/prometheus/targets:ro
      - prometheus-data:/prometheus
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    networks:
      - metrics

networks:
  metrics:
    name: %s

volumes:
  prometheus-data:
    name: %s
`, placement.Network, placement.Volume)
}

func prometheusConfig() string {
	return `global:
  scrape_interval: 5s
  scrape_timeout: 4s

scrape_configs:
  - job_name: baseharbor-applications
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/*.json
        refresh_interval: 2s
    relabel_configs:
      - source_labels: [baseharbor_metrics_path]
        target_label: __metrics_path__
      - action: labeldrop
        regex: baseharbor_metrics_path
`
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
