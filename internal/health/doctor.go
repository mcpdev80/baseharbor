package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"time"

	"github.com/jackc/pgx/v5"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
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
		checks = append(checks, checkCompose(containerRuntime.Name))
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
		checkPostgres(cfg),
		checkOpenBao(cfg.OpenBaoPort),
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

func checkCompose(runtimeName string) Check {
	path, err := exec.LookPath(runtimeName)
	if err != nil {
		return Check{Name: "compose", OK: false, Message: "runtime executable missing"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, path, "compose", "version").Run(); err != nil {
		return Check{Name: "compose", OK: false, Message: runtimeName + " compose unavailable"}
	}
	return Check{Name: "compose", OK: true, Message: runtimeName + " compose available"}
}

func checkPostgres(cfg bhruntime.Config) Check {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connString := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresPort, cfg.PostgresDB)
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return Check{Name: "postgres", OK: false, Message: fmt.Sprintf("connection failed on 127.0.0.1:%d", cfg.PostgresPort)}
	}
	defer conn.Close(ctx)
	var one int
	if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		return Check{Name: "postgres", OK: false, Message: "connected but readiness query failed"}
	}
	return Check{Name: "postgres", OK: true, Message: fmt.Sprintf("query succeeded on 127.0.0.1:%d", cfg.PostgresPort)}
}

func checkOpenBao(port int) Check {
	client := http.Client{Timeout: 3 * time.Second}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	resp, err := client.Get("http://" + address + "/v1/sys/health")
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "not reachable at " + address}
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
		return Check{Name: "openbao", OK: false, Message: "reachable but not initialized"}
	}
	if state.Sealed {
		return Check{Name: "openbao", OK: false, Message: "initialized but sealed"}
	}
	return Check{Name: "openbao", OK: true, Message: "initialized and unsealed"}
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
