package openbao

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func (f *nativeConnectorFixture) sqlVolumeIdentity(t *testing.T, ctx context.Context, project string) string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "podman", "volume", "ls", "-q", "--filter", "label=com.docker.compose.project="+project).Output()
	names := strings.Fields(string(output))
	if err != nil || len(names) != 1 {
		t.Fatal("owned SQL provider volume is missing or ambiguous", err)
	}
	output, err = exec.CommandContext(ctx, "podman", "volume", "inspect", "--format", "{{.Name}}|{{.CreatedAt}}|{{.Mountpoint}}", names[0]).Output()
	identity := strings.TrimSpace(string(output))
	if err != nil || len(strings.Split(identity, "|")) != 3 || strings.Contains(identity, "||") {
		t.Fatal("native SQL volume identity is unavailable", err)
	}
	return identity
}

func (f *nativeConnectorFixture) reapplyRetainedSQL(t *testing.T, ctx context.Context, managed *application.RemoteManagedRuntime, project, identity string, verify func()) {
	t.Helper()
	if identity == "" || f.sqlVolumeIdentity(t, ctx, project) != identity {
		t.Fatal("ordinary SQL destroy replaced or removed provider data")
	}
	if err := managed.Apply(ctx, true); err != nil {
		t.Fatal("owned SQL reapply after ordinary destroy failed", err)
	}
	verify()
	if f.sqlVolumeIdentity(t, ctx, project) != identity {
		t.Fatal("SQL reapply created a second provider volume")
	}
	if err := managed.Destroy(ctx); err != nil {
		t.Fatal("reapplied owned SQL teardown failed", err)
	}
	services, err := managed.Observe(ctx)
	if err != nil || len(services) != 0 || f.sqlVolumeIdentity(t, ctx, project) != identity {
		t.Fatal("reapplied SQL teardown changed owned provider data", err)
	}
	t.Log("ordinary generated SQL destroy/reapply reused the exact owned data volume and verified TLS SELECT 1; explicit owned reset not qualified")
}
