package providerbinding

import (
	"context"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryInventoryAllowsMissingFailedContainerOnlyWithProtectedPinnedSource(t *testing.T) {
	oldDigest := "sha256:" + strings.Repeat("a", 64)
	newDigest := "sha256:" + strings.Repeat("b", 64)
	path := filepath.Join(t.TempDir(), "compose.yaml")
	req := providerupgrade.Request{CurrentVersion: "2.7.0", TargetVersion: "2.7.1", TargetImage: "docker.io/openbao/openbao:2.7.1", TargetDigest: newDigest}
	reader := &fakeReader{}
	binding := &RuntimeBinding{Reader: reader, Engine: "docker", Sources: map[providerupgrade.Provider]ManagedSource{providerupgrade.ProviderOpenBao: {Project: "owned-core", Service: "openbao-member-1", Compose: path, OriginalImage: "docker.io/openbao/openbao:2.7.0", OriginalDigest: oldDigest}}}
	write := func(image string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte("services:\n  openbao-member-1:\n    image: "+image+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	write(req.TargetImage+"@"+newDigest, 0600)
	identity, err := binding.InspectRecovery(context.Background(), providerupgrade.ProviderOpenBao, req)
	if err != nil || !identity.Owned || identity.Project != "owned-core" || identity.Digest != newDigest {
		t.Fatalf("verified retained recovery source refused: %#v %v", identity, err)
	}
	// Normal upgrade selection still requires a live native provider.
	if _, err := binding.InspectManaged(context.Background(), providerupgrade.ProviderOpenBao); err == nil {
		t.Fatal("missing runtime accepted for normal upgrade")
	}
	for _, image := range []string{req.TargetImage, "foreign/provider:2.7.1@" + newDigest, req.TargetImage + "@sha256:" + strings.Repeat("c", 64)} {
		write(image, 0600)
		if _, err := binding.InspectRecovery(context.Background(), providerupgrade.ProviderOpenBao, req); err == nil {
			t.Fatalf("unverified retained image accepted: %s", image)
		}
	}
	write(req.TargetImage+"@"+newDigest, 0644)
	if _, err := binding.InspectRecovery(context.Background(), providerupgrade.ProviderOpenBao, req); err == nil {
		t.Fatal("unprotected recovery source accepted")
	}
	write(req.TargetImage+"@"+newDigest, 0600)
	reader.containers = []bhruntime.RuntimeContainer{{Project: "owned-core", Service: "openbao-member-1", Running: false}}
	reader.identity = bhruntime.ImageIdentity{Reference: "foreign/provider:2.7.1", Digest: newDigest}
	if _, err := binding.InspectRecovery(context.Background(), providerupgrade.ProviderOpenBao, req); err == nil {
		t.Fatal("foreign native replacement accepted")
	}
	reader.identity = bhruntime.ImageIdentity{Reference: req.TargetImage, Digest: newDigest}
	if _, err := binding.InspectRecovery(context.Background(), providerupgrade.ProviderOpenBao, req); err != nil {
		t.Fatalf("owned stopped member cannot be recovered: %v", err)
	}
}
