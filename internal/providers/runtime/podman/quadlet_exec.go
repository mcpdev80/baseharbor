package podman

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func quadletExecAs(ctx context.Context, runtimeCommand, container, user string, input []byte, args ...string) (string, error) {
	full := []string{"exec"}
	if input != nil {
		full = append(full, "-i")
	}
	full = append(full, "--user", user, container)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, runtimeCommand, full...)
	cmd.Env = runtimeCommandEnv(runtimeCommand)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("podman exec %s: %s", container, message)
	}
	return stdout.String(), nil
}
