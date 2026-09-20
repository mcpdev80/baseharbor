package traces

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

const (
	ProviderImage   = "grafana/tempo:2.7.2"
	ProviderService = "tempo"
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	StopProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type Placement struct {
	Scope            capability.ProviderScope
	Project          string
	Network          string
	Volume           string
	Dir              string
	SharingBoundary  string
	OwnerApplication string
}

type ProviderFiles struct {
	Dir     string
	Compose string
	Env     string
	Config  string
}

type Driver struct {
	runtime Runtime
	app     application.Manifest
}

func NewDriver(runtime Runtime, app application.Manifest) *Driver {
	return &Driver{runtime: runtime, app: app}
}

func (d *Driver) Descriptor() capability.Provider { return capability.Tempo }

func (d *Driver) Preflight(_ context.Context, resource capability.Resource, _ capability.Binding) error {
	if resource.Kind != capability.Traces {
		return fmt.Errorf("Tempo provider cannot satisfy %s", resource.Kind)
	}
	enabled, err := application.TracesCollectionEnabled(d.app)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("trace storage is disabled by deployment policy")
	}
	if !application.HasTraceSignal(d.app) {
		return errors.New("trace storage requires an application OTLP traces signal")
	}
	placement, err := application.ResolveProviderPlacement(d.app, capability.ProviderTempo)
	if err != nil {
		return err
	}
	if placement.Scope == capability.ScopeExternal {
		return errors.New("external Tempo placement requires an external trace storage adapter")
	}
	if application.TelemetryProviderForDeployment().Kind != capability.ProviderOTelCollector {
		return errors.New("managed Tempo requires the managed OpenTelemetry Collector transport")
	}
	return nil
}

func (d *Driver) Provision(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	_, err := Provision(ctx, d.runtime, d.app)
	return err
}

func (d *Driver) Bind(context.Context, capability.Resource, capability.Binding) error { return nil }

func (d *Driver) Verify(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	return VerifyTrace(ctx, d.app, telemetry.ProbeTraceIDHex)
}

func PlacementFor(m application.Manifest) (Placement, error) {
	p, err := application.ResolveProviderPlacement(m, capability.ProviderTempo)
	if err != nil {
		return Placement{}, err
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Placement{}, err
	}
	switch p.Scope {
	case capability.ScopeShared:
		project := "baseharbor-traces"
		network := "baseharbor-traces"
		volume := "baseharbor-tempo-data"
		dir := filepath.Join(dataDir, "providers", "tempo", "shared")
		if p.SharingBoundary != "" {
			token := application.ProviderPlacementNameToken(p.SharingBoundary)
			project += "-" + token
			network += "-" + token
			volume += "-" + token
			dir = filepath.Join(dir, token)
		}
		return Placement{Scope: p.Scope, Project: project, Network: network, Volume: volume, Dir: dir, SharingBoundary: p.SharingBoundary}, nil
	case capability.ScopeApplication:
		suffix := m.Name + "-" + m.Environment
		return Placement{Scope: p.Scope, Project: "baseharbor-traces-" + suffix, Network: "baseharbor-traces-" + suffix, Volume: "baseharbor-tempo-data-" + suffix, Dir: filepath.Join(dataDir, "providers", "tempo", "applications", m.Name, m.Environment), OwnerApplication: m.Name}, nil
	case capability.ScopeExternal:
		return Placement{Scope: p.Scope}, nil
	default:
		return Placement{}, fmt.Errorf("unsupported Tempo provider scope %q", p.Scope)
	}
}

func EnsureProviderFiles(m application.Manifest) (ProviderFiles, Placement, error) {
	p, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, Placement{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, p, errors.New("external Tempo adapter is not implemented")
	}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return ProviderFiles{}, p, err
	}
	files := ProviderFiles{Dir: p.Dir, Compose: filepath.Join(p.Dir, "compose.yaml"), Env: filepath.Join(p.Dir, "runtime.env"), Config: filepath.Join(p.Dir, "tempo.yaml")}
	port := ""
	if data, err := os.ReadFile(files.Env); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == "BASEHARBOR_TEMPO_PORT" {
				port = strings.TrimSpace(v)
			}
		}
	}
	if port == "" {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return ProviderFiles{}, p, err
		}
		port = strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
		_ = ln.Close()
	}
	if err := os.WriteFile(files.Env, []byte("BASEHARBOR_TEMPO_PORT="+port+"\n"), 0o600); err != nil {
		return ProviderFiles{}, p, err
	}
	if err := os.WriteFile(files.Config, []byte(configYAML()), 0o644); err != nil {
		return ProviderFiles{}, p, err
	}
	if err := os.WriteFile(files.Compose, []byte(composeYAML(p)), 0o600); err != nil {
		return ProviderFiles{}, p, err
	}
	return files, p, nil
}

func ExistingProviderFiles(m application.Manifest) (ProviderFiles, Placement, error) {
	p, err := PlacementFor(m)
	if err != nil {
		return ProviderFiles{}, Placement{}, err
	}
	if p.Scope == capability.ScopeExternal {
		return ProviderFiles{}, p, os.ErrNotExist
	}
	files := ProviderFiles{Dir: p.Dir, Compose: filepath.Join(p.Dir, "compose.yaml"), Env: filepath.Join(p.Dir, "runtime.env"), Config: filepath.Join(p.Dir, "tempo.yaml")}
	for _, path := range []string{files.Compose, files.Env, files.Config} {
		if _, err := os.Stat(path); err != nil {
			return ProviderFiles{}, p, err
		}
	}
	return files, p, nil
}

func Provision(ctx context.Context, runtime Runtime, m application.Manifest) (Placement, error) {
	files, p, err := EnsureProviderFiles(m)
	if err != nil {
		return Placement{}, err
	}
	if err := runtime.ConfigProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return Placement{}, err
	}
	if err := runtime.UpProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return Placement{}, err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return Placement{}, err
	}
	if err := waitReady(ctx, endpoint); err != nil {
		return Placement{}, err
	}
	class := observability.SourcePlatformProvider
	if p.Scope == capability.ScopeApplication {
		class = observability.SourceApplicationProvider
	}
	if err := observability.Update(observability.MetricsSource{
		ID: "tempo:" + p.Project, Provider: capability.ProviderTempo, Class: class, Scope: p.Scope,
		SharingBoundary: p.SharingBoundary, OwnerApplication: p.OwnerApplication,
		Network: p.Network, Target: "tempo:3200", Path: "/metrics",
	}); err != nil {
		return Placement{}, err
	}
	return p, nil
}

func StopProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	p, err := PlacementFor(m)
	if err != nil || p.Scope != capability.ScopeApplication {
		return err
	}
	files := ProviderFiles{Dir: p.Dir, Compose: filepath.Join(p.Dir, "compose.yaml"), Env: filepath.Join(p.Dir, "runtime.env"), Config: filepath.Join(p.Dir, "tempo.yaml")}
	if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return runtime.StopProject(ctx, p.Project, files.Compose, files.Env)
}

func DestroyProvider(ctx context.Context, runtime Runtime, m application.Manifest) error {
	p, err := PlacementFor(m)
	if err != nil || p.Scope == capability.ScopeExternal {
		return err
	}
	files := ProviderFiles{Dir: p.Dir, Compose: filepath.Join(p.Dir, "compose.yaml"), Env: filepath.Join(p.Dir, "runtime.env"), Config: filepath.Join(p.Dir, "tempo.yaml")}
	if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := runtime.DestroyProject(ctx, p.Project, files.Compose, files.Env); err != nil {
		return err
	}
	_ = observability.Remove("tempo:" + p.Project)
	return os.RemoveAll(p.Dir)
}

func DestroyAllSharedProviders(ctx context.Context, runtime Runtime) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	root := filepath.Join(dataDir, "providers", "tempo", "shared")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	destroyAt := func(dir, project string) error {
		files := ProviderFiles{Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), Config: filepath.Join(dir, "tempo.yaml")}
		if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		if err := runtime.DestroyProject(ctx, project, files.Compose, files.Env); err != nil {
			return err
		}
		_ = observability.Remove("tempo:" + project)
		return nil
	}
	if err := destroyAt(root, "baseharbor-traces"); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := destroyAt(filepath.Join(root, entry.Name()), "baseharbor-traces-"+entry.Name()); err != nil {
			return err
		}
	}
	return os.RemoveAll(root)
}

func NetworkEndpoint(p Placement) string { return "http://tempo:4318" }

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == "BASEHARBOR_TEMPO_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Tempo port")
			}
			return fmt.Sprintf("http://127.0.0.1:%d", port), nil
		}
	}
	return "", errors.New("Tempo port is missing")
}

func VerifyTrace(ctx context.Context, m application.Manifest, traceID string) error {
	files, _, err := EnsureProviderFiles(m)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		req, _ := http.NewRequestWithContext(deadline, http.MethodGet, strings.TrimRight(endpoint, "/")+"/api/traces/"+traceID, nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("Tempo did not return verification trace %s before timeout", traceID)
		case <-ticker.C:
		}
	}
}

func waitReady(ctx context.Context, endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/ready", nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func configYAML() string {
	return `server:
  http_listen_port: 3200
distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: 0.0.0.0:4318
storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/traces
`
}

func composeYAML(p Placement) string {
	return fmt.Sprintf(`services:
  tempo:
    image: %s
    restart: unless-stopped
    user: "10001:10001"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    command: ["-config.file=/etc/tempo/tempo.yaml"]
    volumes:
      - ./tempo.yaml:/etc/tempo/tempo.yaml:ro
      - tempo-data:/var/tempo
    ports:
      - "127.0.0.1:${BASEHARBOR_TEMPO_PORT}:3200"
    networks:
      traces:
        aliases: [tempo]
networks:
  traces:
    name: %q
volumes:
  tempo-data:
    name: %q
`, ProviderImage, p.Network, p.Volume)
}
