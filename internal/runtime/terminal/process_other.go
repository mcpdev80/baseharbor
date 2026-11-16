//go:build !linux

package terminal

import (
	"context"
	"errors"
	"os/exec"
)

func Start(context.Context, *exec.Cmd, int, int) (Session, error) {
	return nil, errors.New("runtime terminal is currently supported on Linux only")
}
