package logs_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/providerconformance"
)

type fakeRuntime struct {
	mu       sync.Mutex
	server   *http.Server
	listener net.Listener
}

func (r *fakeRuntime) ConfigProject(context.Context, string, string, string) error { return nil }

func (r *fakeRuntime) UpProject(_ context.Context, _ string, _ string, envFile string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		return nil
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		return err
	}
	var port int
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && key == "BASEHARBOR_LOKI_PORT" {
			port, err = strconv.Atoi(value)
			if err != nil {
				return err
			}
		}
	}
	if port == 0 {
		return fmt.Errorf("missing Loki test port")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/loki/api/v1/query_range", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"baseharbor_application":"demo","baseharbor_environment":"dev","baseharbor_service":"api"},"values":[["1","ready"]]}]}}`))
	})
	r.listener = listener
	r.server = &http.Server{Handler: mux}
	go func() { _ = r.server.Serve(listener) }()
	return nil
}

func (r *fakeRuntime) StopProject(context.Context, string, string, string) error    { return nil }
func (r *fakeRuntime) DestroyProject(context.Context, string, string, string) error { return nil }

func (r *fakeRuntime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		_ = r.server.Close()
	}
	if r.listener != nil {
		_ = r.listener.Close()
	}
}

func TestLokiDriverConsumesProviderConformanceHarness(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.LogsEnabledEnv, "true")
	runtime := &fakeRuntime{}
	defer runtime.Close()
	m := application.New("demo", "dev", false, false, false)
	driver := logs.NewDriver(runtime, m)
	target := providerconformance.Target{
		Descriptor:       capability.LokiIntegration,
		Application:      m.Name,
		UnsupportedScope: capability.ScopeExternal,
		Request: capability.Request{
			Requirement: capability.Requirement{Kind: capability.Logs, Name: "api"},
			Workload:    "service/api",
			Logs:        &capability.LogsBinding{Direction: "collect", Format: "syslog-rfc5424", Service: "api"},
			Driver:      driver,
		},
	}
	report := providerconformance.Run(context.Background(), target)
	if err := providerconformance.Require(report); err != nil {
		t.Fatalf("Loki conformance: %v (%#v)", err, report)
	}
}

func TestLokiProviderRuntimeDoesNotMountContainerSocket(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("demo", "dev", false, false, false)
	files, err := logs.EnsureProviderFiles(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	compose := string(data)
	for _, forbidden := range []string{"docker.sock", "podman.sock", "privileged: true", "network_mode: host"} {
		if strings.Contains(compose, forbidden) {
			t.Fatalf("provider runtime contains forbidden isolation bypass %q:\n%s", forbidden, compose)
		}
	}
	for _, required := range []string{"read_only: true", "cap_drop:", "no-new-privileges:true", "127.0.0.1:"} {
		if !strings.Contains(compose, required) {
			t.Fatalf("provider runtime missing hardening %q:\n%s", required, compose)
		}
	}
}

func TestWorkloadLoggingOverrideUsesLoopbackSyslog(t *testing.T) {
	state := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", state)
	m := application.New("demo", "dev", false, false, false)
	if _, err := logs.EnsureProviderFiles(m); err != nil {
		t.Fatal(err)
	}
	runtime := application.RuntimeFiles{Dir: t.TempDir()}
	path, err := logs.EnsureWorkloadOverride(m, runtime, []string{"api", "worker"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	override := string(data)
	for _, required := range []string{"driver: syslog", "udp://127.0.0.1:", "syslog-format: rfc5424", "tag: \"api\"", "tag: \"worker\""} {
		if !strings.Contains(override, required) {
			t.Fatalf("override missing %q:\n%s", required, override)
		}
	}
}


func TestLokiConfigKeepsWALOnWritablePersistentVolume(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("demo", "dev", false, false, false)
	files, err := logs.EnsureProviderFiles(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.LokiConfig)
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, required := range []string{
		"ingester:",
		"wal:",
		"enabled: true",
		"dir: /loki/wal",
	} {
		if !strings.Contains(config, required) {
			t.Fatalf("Loki config missing writable WAL setting %q:\n%s", required, config)
		}
	}
	if strings.Contains(config, "dir: wal") {
		t.Fatalf("Loki WAL must not use a relative path with read-only root filesystem:\n%s", config)
	}
}


func TestManagedProviderFilesSupportNonRootContainerReaders(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("demo", "dev", false, false, false)
	files, err := logs.EnsureProviderFiles(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.LokiConfig, files.AlloyConfig} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Fatalf("%s mode=%#o want 0644 for non-root container bind-mount reads", path, got)
		}
	}
	for _, path := range []string{files.Env, files.Registrations, files.Compose} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%#o want 0600 for host-only provider state", path, got)
		}
	}
}

func TestLokiProviderInitializesNamedVolumesForRuntimeUIDs(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	m := application.New("demo", "dev", false, false, false)
	files, err := logs.EnsureProviderFiles(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	compose := string(data)
	for _, required := range []string{
		"provider-volume-init:",
		"image: busybox:1.37",
		"user: \"0:0\"",
		"cap_add: [\"CHOWN\"]",
		"chown -R 10001:10001 /loki",
		"chown -R 473:473 /var/lib/alloy/data",
		"user: \"10001:10001\"",
		"user: \"473:473\"",
		"condition: service_completed_successfully",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("provider Compose missing non-root volume initialization %q:\n%s", required, compose)
		}
	}
}
