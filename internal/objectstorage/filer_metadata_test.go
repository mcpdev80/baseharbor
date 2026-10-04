package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type metadataReplicationRuntime struct {
	stdinCaptureRuntime
	incomplete string
	rejoined   []string
	rejoinErr  error
	queries    []string
}

func (r *metadataReplicationRuntime) ExecProjectInput(_ context.Context, _, _, _ string, input []byte, member string, args ...string) (string, error) {
	if len(args) != 3 || args[2] != "-filer="+member+":8888" {
		return "", errors.New("metadata probe was not pinned to its reader")
	}
	if !strings.Contains(string(input), "fs.meta.cat") {
		return "", nil
	}
	r.queries = append(r.queries, member)
	var out strings.Builder
	for _, source := range filerMembers {
		if member == r.incomplete && source == "seaweedfs-node-3" {
			continue
		}
		fmt.Fprintf(&out, "{\"name\": \"%s\"}\n", source)
	}
	return out.String(), nil
}

func (r *metadataReplicationRuntime) UpProjectFilesSelectedForceRecreateNoBuild(_ context.Context, _, _ string, _ map[string]string, members []string, _ ...string) error {
	if r.rejoinErr != nil {
		return r.rejoinErr
	}
	r.rejoined = append(r.rejoined, members...)
	if reflect.DeepEqual(members, []string{r.incomplete}) {
		r.incomplete = ""
	}
	return nil
}

func metadataProbeFiles(t *testing.T) ProviderFiles {
	t.Helper()
	dir := t.TempDir()
	env := filepath.Join(dir, "runtime.env")
	if err := os.WriteFile(env, []byte("PROBE=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ProviderFiles{Dir: dir, Env: env, Compose: filepath.Join(dir, "compose.yaml"), Project: "metadata-proof"}
}

func TestFilerMetadataProofDetectsReachableReaderWithMissingPeer(t *testing.T) {
	runtime := &metadataReplicationRuntime{incomplete: "seaweedfs-node-2"}
	files := metadataProbeFiles(t)
	ctx := context.Background()
	missing := missingFilerMetadataMarkers(ctx, runtime, files, "/etc/baseharbor/ha-probes/test")
	if !reflect.DeepEqual(missing, []string{"seaweedfs-node-2"}) {
		t.Fatalf("incomplete readers = %v", missing)
	}
	if err := rejoinFilerMetadataMembers(ctx, runtime, files, "/etc/baseharbor/ha-probes/test", missing); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runtime.rejoined, missing) {
		t.Fatalf("rejoined members = %v, want only %v", runtime.rejoined, missing)
	}
	if missing := missingFilerMetadataMarkers(ctx, runtime, files, "/etc/baseharbor/ha-probes/test"); len(missing) != 0 {
		t.Fatalf("metadata proof still fails after rejoin: %v", missing)
	}
}

func TestFilerMetadataProofDoesNotRecreateHealthyMembers(t *testing.T) {
	runtime := &metadataReplicationRuntime{}
	if err := reconcileFilerMetadata(context.Background(), runtime, metadataProbeFiles(t)); err != nil {
		t.Fatal(err)
	}
	if len(runtime.rejoined) != 0 || !reflect.DeepEqual(runtime.queries, filerMembers) {
		t.Fatalf("healthy cluster: rejoined=%v, readers=%v", runtime.rejoined, runtime.queries)
	}
}

func TestFilerMetadataRejoinFailureIsNotAccepted(t *testing.T) {
	runtime := &metadataReplicationRuntime{incomplete: "seaweedfs-node-2", rejoinErr: errors.New("rejoin failed")}
	err := rejoinFilerMetadataMembers(context.Background(), runtime, metadataProbeFiles(t), "/etc/baseharbor/ha-probes/test", []string{runtime.incomplete})
	if err == nil || !strings.Contains(err.Error(), "rejoin failed") {
		t.Fatalf("rejoin failure = %v", err)
	}
}
