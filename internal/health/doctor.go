package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Check struct {
	Name    string
	OK      bool
	Message string
}

func Doctor() []Check {
	checks := []Check{
		{Name: "os", OK: runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows", Message: runtime.GOOS + "/" + runtime.GOARCH},
	}

	containerRuntime, runtimeOK := checkContainerRuntime()
	checks = append(checks, containerRuntime)
	if runtimeOK {
		checks = append(checks, checkCompose(containerRuntime.Name))
	}

	if runtimeStateExists() {
		checks = append(checks, checkTCP("postgres", "127.0.0.1:5432"))
		checks = append(checks, checkOpenBao())
	}

	return checks
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

func runtimeStateExists() bool {
	_, err := os.Stat(filepath.Join(".baseharbor", "runtime", "runtime.env"))
	return err == nil
}

func checkTCP(name, address string) Check {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return Check{Name: name, OK: false, Message: "not reachable at " + address}
	}
	_ = conn.Close()
	return Check{Name: name, OK: true, Message: "reachable at " + address}
}

func checkOpenBao() Check {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:8200/v1/sys/health")
	if err != nil {
		return Check{Name: "openbao", OK: false, Message: "not reachable at 127.0.0.1:8200"}
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
