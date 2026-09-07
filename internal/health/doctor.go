package health

import (
	"fmt"
	"os/exec"
	"runtime"
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

	if path, err := exec.LookPath("docker"); err == nil {
		checks = append(checks, Check{Name: "docker", OK: true, Message: path})
	} else if path, err := exec.LookPath("podman"); err == nil {
		checks = append(checks, Check{Name: "podman", OK: true, Message: path})
	} else {
		checks = append(checks, Check{Name: "container-runtime", OK: false, Message: "docker or podman not found"})
	}

	return checks
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
