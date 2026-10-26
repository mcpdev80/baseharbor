package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// This acceptance creates and removes only its uniquely named resources.
func TestRenovateVolumeRecoveryRuntimeAcceptance(t *testing.T) {
	if os.Getenv("BASEHARBOR_RENOVATE_RECOVERY_ACCEPTANCE") != "1" {
		t.Skip("requires isolated rootless Docker/Podman recovery acceptance")
	}
	engine := os.Getenv("BASEHARBOR_TEST_RUNTIME")
	if engine != "docker" && engine != "podman" {
		t.Fatal("BASEHARBOR_TEST_RUNTIME must explicitly select docker or podman")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := NewCLIBackend(engine)
	command := func(args ...string) string {
		t.Helper()
		out, err := c.DirectOutput(ctx, args...)
		if err != nil {
			t.Fatalf("%s %s: %v", engine, strings.Join(args, " "), err)
		}
		return strings.TrimSpace(out)
	}
	if engine == "podman" {
		if command("info", "--format", "{{.Host.Security.Rootless}}") != "true" {
			t.Fatal("acceptance requires rootless Podman")
		}
	} else if !strings.Contains(command("info", "--format", "{{json .SecurityOptions}}"), "rootless") {
		t.Fatal("acceptance requires rootless Docker")
	}
	t.Logf("engine=%s helper=%s", engine, recoveryHelperImage)
	project := fmt.Sprintf("baha-renovate-%d", time.Now().UnixNano())
	source, restored, invalid, foreign := project+"-source", project+"-restore", project+"-invalid", project+"-foreign"
	consumer := project + "-consumer"
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		_, _ = c.DirectOutput(cleanup, "container", "rm", "-f", consumer)
		for _, volume := range []string{source, restored, invalid, foreign} {
			if _, err := c.DirectOutput(cleanup, "volume", "rm", volume); err != nil {
				t.Errorf("remove test volume %s: %v", volume, err)
			}
		}
	})
	for _, volume := range []string{source, restored, invalid} {
		if err := c.EnsureOwnedVolume(ctx, project, volume); err != nil {
			t.Fatal(err)
		}
	}
	command("volume", "create", "--label", "com.docker.compose.project="+project+"-other", "--label", "io.podman.compose.project="+project+"-other", foreign)
	command("run", "--rm", "-v", source+":/data", recoveryHelperImage, "sh", "-ceu", "mkdir /data/private; printf 'recovery payload' > /data/private/payload; chmod 0750 /data/private; chmod 0640 /data/private/payload; chown -R 107:103 /data/private; ln -s private/payload /data/link")
	command("run", "--rm", "-v", foreign+":/data", recoveryHelperImage, "sh", "-ceu", "printf 'protected sentinel' > /data/sentinel")
	archive, err := c.ExportOwnedVolume(ctx, project, source)
	if err != nil {
		t.Fatal(err)
	}
	assertArchive := func(archive []byte) {
		t.Helper()
		r := tar.NewReader(bytes.NewReader(archive))
		found := map[string]bool{}
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
			switch name {
			case "private", "private/payload":
				mode := int64(0750)
				if name == "private/payload" {
					mode = 0640
					data, err := io.ReadAll(r)
					if err != nil || string(data) != "recovery payload" {
						t.Fatalf("payload changed: %q %v", data, err)
					}
				}
				if h.Uid != 107 || h.Gid != 103 || h.Mode&0777 != mode {
					t.Fatalf("ownership/permissions changed: %+v", h)
				}
				found[name] = true
			case "link":
				if h.Typeflag != tar.TypeSymlink || h.Linkname != "private/payload" {
					t.Fatalf("symlink changed: %+v", h)
				}
				found[name] = true
			}
		}
		if len(found) != 3 {
			t.Fatalf("incomplete archive: %v", found)
		}
	}
	assertArchive(archive)
	command("run", "--rm", "-v", restored+":/data", recoveryHelperImage, "sh", "-ceu", "printf stale > /data/stale")
	if err := c.RestoreOwnedVolume(ctx, project, restored, archive); err != nil {
		t.Fatal(err)
	}
	command("run", "--rm", "-v", restored+":/data:ro", recoveryHelperImage, "sh", "-ceu", "test ! -e /data/stale")
	result, err := c.ExportOwnedVolume(ctx, project, restored)
	if err != nil {
		t.Fatal(err)
	}
	assertArchive(result)
	if _, err := c.ExportOwnedVolume(ctx, project, foreign); err == nil {
		t.Fatal("foreign volume export accepted")
	}
	if err := c.RestoreOwnedVolume(ctx, project, foreign, archive); err == nil {
		t.Fatal("foreign volume restore accepted")
	}
	if err := c.RestoreOwnedVolume(ctx, project, invalid, []byte("invalid tar archive")); err == nil {
		t.Fatal("corrupt archive reported success")
	}
	command("run", "-d", "--name", consumer, "--label", "com.docker.compose.project="+project+"-other", "-v", source+":/data:ro", recoveryHelperImage, "sleep", "120")
	if err := c.VerifyOwnedVolumeQuiesced(ctx, project, source); err == nil {
		t.Fatal("shared active volume reported quiesced")
	}
	if command("container", "inspect", "--format", "{{.State.Running}}", consumer) != "true" {
		t.Fatal("foreign consumer changed")
	}
	if got := command("run", "--rm", "-v", foreign+":/data:ro", recoveryHelperImage, "cat", "/data/sentinel"); got != "protected sentinel" {
		t.Fatal("foreign recovery data changed")
	}
	command("container", "rm", "-f", consumer)
	if err := c.VerifyOwnedVolumeQuiesced(ctx, project, source); err != nil {
		t.Fatal(err)
	}
	// Both adoption formats pin exactly this same multiarchitecture index.
	const valkey = "valkey/valkey:8@sha256:640c5e62cea04b6d6f2084232651d0cc70362d31f4f805e7be94dbed6855e8f2"
	if !strings.Contains(command("run", "--rm", valkey, "valkey-server", "--version"), "v=8.") {
		t.Fatal("pinned adoption image does not report Valkey 8")
	}
}
