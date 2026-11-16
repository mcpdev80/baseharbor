package podman

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func quadletSystemctl(ctx context.Context, input []byte, args ...string) (string, error) {
	return quadletSystemctlMode(ctx, input, true, args...)
}

func quadletSystemctlBlocking(ctx context.Context, input []byte, args ...string) (string, error) {
	return quadletSystemctlMode(ctx, input, false, args...)
}

func quadletSystemctlMode(ctx context.Context, input []byte, noBlock bool, args ...string) (string, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return "", err
	}
	full := append([]string{"--user"}, args...)
	if noBlock && len(args) > 0 && (args[0] == "start" || args[0] == "restart") {
		full = append([]string{"--user", "--no-block"}, args...)
	}
	cmd := exec.CommandContext(ctx, path, full...)
	cmd.Env = quadletUserRuntimeEnv()
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
		return stdout.String(), fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
