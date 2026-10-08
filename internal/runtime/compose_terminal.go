package runtime

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

func (c Compose) ContainerTerminal(ctx context.Context, id string, argv []string, rows, columns int) (terminal.Session, error) {
	if c.command == "" {
		return nil, ErrRuntimeNotFound
	}
	if strings.TrimSpace(id) == "" || strings.HasPrefix(id, "-") || strings.ContainsRune(id, 0) || len(argv) == 0 {
		return nil, errors.New("terminal requires a stable container ID and explicit argv")
	}
	for _, arg := range argv {
		if strings.ContainsRune(arg, 0) {
			return nil, errors.New("terminal argv contains NUL")
		}
	}
	args := append([]string{"container", "exec", "--interactive", "--tty", id}, argv...)
	cmd := exec.CommandContext(ctx, c.command, args...)
	cmd.Env = runtimeCommandEnv(c.command)
	return terminal.Start(ctx, cmd, rows, columns)
}
