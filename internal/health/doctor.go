package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	"github.com/jackc/pgx/v5"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type Check struct {
	Name    string
	OK      bool
	Message string
}

func Doctor() []Check {
	checks := []Check{
		{Name: "os", OK: goruntime.GOOS == "linux" || goruntime.GOOS == "darwin" || goruntime.GOOS == "windows", Message: goruntime.GOOS + "/" + goruntime.GOARCH},
	}

	containerRuntime, runtimeOK := checkContainerRuntime()
	checks = append(checks, containerRuntime)
	if runtimeOK {
		checks = append(checks, checkRuntimeOrchestration(containerRuntime.Name))
	}
	checks = append(checks, RuntimeChecks()...)
	return checks
}

// RuntimeChecks reports actual service readiness once BaseHarbor runtime state
// exists. A running but sealed/uninitialized OpenBao is intentionally not OK.
func RuntimeChecks() []Check {
	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Check{{Name: "runtime-config", OK: false, Message: "runtime state is unreadable"}}
	}
	cfg, err := bhruntime.LoadConfig(files.Env)
	if err != nil {
		return []Check{{Name: "runtime-config", OK: false, Message: "runtime configuration is invalid"}}
	}
	return []Check{
		checkPostgres(cfg, files),
		checkOpenBao(cfg.OpenBaoPort, files),
	}
}

func checkContainerRuntime() (Check, bool) {
	for _, name := range []string{"docker", "podman"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, path, "info")
		err = cmd.Run()
		cancel()
		if err == nil {
			return Check{Name: name, OK: true, Message: "daemon reachable"}, true
		}
	}
	return Check{Name: "container-runtime", OK: false, Message: "docker/podman daemon not reachable"}, false
}

func checkRuntimeOrchestration(runtimeName string) Check {
	path, err := exec.LookPath(runtimeName)
	if err != nil {
		return Check{Name: "runtime-orchestration", OK: false, Message: "runtime executable missing"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if runtimeName == "podman" {
		if !bhruntime.QuadletAvailable(ctx) {
			return Check{Name: "quadlet", OK: false, Message: "Podman Quadlet unavailable"}
		}
		return Check{Name: "quadlet", OK: true, Message: "Podman Quadlet available"}
	}
	if err := exec.CommandContext(ctx, path, "compose", "version").Run(); err != nil {
		return Check{Name: "compose", OK: false, Message: runtimeName + " compose unavailable"}
	}
	return Check{Name: "compose", OK: true, Message: runtimeName + " compose available"}
}

func checkPostgres(cfg bhruntime.Config, files bhruntime.Files) Check {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	policy, err := serviceaccess.Resolve("prod", "control-plane-postgresql", serviceaccess.AuthenticationNative)
	if err != nil {
		return Check{Name: "postgres", OK: false, Message: "TLS policy is invalid"}
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(filepath.Dir(files.Compose), "providers", "postgresql", "service-access", "pki"))
	if err != nil {
		return Check{Name: "postgres", OK: false, Message: "TLS material is unavailable"}
	}
	connString := fmt.Sprintf(
		"postgres://%s:%s@127.0.0.1:%d/%s?sslmode=verify-full&sslrootcert=%s",
		url.QueryEscape(cfg.PostgresUser),
		url.QueryEscape(cfg.PostgresPassword),
		cfg.PostgresPort,
		url.PathEscape(cfg.PostgresDB),
		url.QueryEscape(material.CA),
	)
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return Check{Name: "postgres", OK: false, Message: fmt.Sprintf("TLS connection failed on 127.0.0.1:%d", cfg.PostgresPort)}
	}
	defer conn.Close(ctx)
	var one int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		return Check{Name: "postgres", OK: false, Message: "TLS connected but readiness query failed"}
	}
	return Check{Name: "postgres", OK: true, Message: fmt.Sprintf("verified TLS query succeeded on 127.0.0.1:%d", cfg.PostgresPort)}
}

func checkOpenBao(port int, files bhruntime.Files) Check {
	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "TLS policy is invalid"}
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(filepath.Dir(files.Compose), "providers", "openbao", "service-access", "pki"))
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "TLS material is unavailable"}
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "TLS client configuration is invalid"}
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	resp, err := client.Get("https://" + address + "/v1/sys/health")
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "not reachable via verified HTTPS at " + address}
	}
	defer resp.Body.Close()

	var state struct {
		Initialized bool `json:"initialized"`
		Sealed      bool `json:"sealed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return Check{Name: "openbao", OK: false, Message: "health response is invalid"}
	}
	if !state.Initialized {
		return Check{Name: "openbao", OK: false, Message: "not initialized"}
	}
	if state.Sealed {
		return Check{Name: "openbao", OK: false, Message: "sealed"}
	}
	return Check{Name: "openbao", OK: true, Message: "initialized, unsealed and reachable via verified HTTPS"}
}

func Format(checks []Check) (string, bool) {
	ok := true
	out := ""
	for _, check := range checks {
		mark := "OK"
		if !check.OK {
			mark = "FAIL"
			ok = false
		}
		out += fmt.Sprintf("[%s] %-18s %s\n", mark, check.Name, check.Message)
	}
	return out, ok
}
