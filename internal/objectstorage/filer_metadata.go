package objectstorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

var filerMembers = []string{"seaweedfs-node-1", "seaweedfs-node-2", "seaweedfs-node-3"}

// SeaweedFS can lose a peer join between its initial peer snapshot and the
// metadata subscription callback registration. A reachable filer then has an
// incomplete local metadata store, even though best-effort IAM cache pushes
// temporarily make its S3 endpoint appear healthy. Prove every directed
// metadata replication path before configuring identities. Rejoin only a
// reader that fails this proof, then require the same proof to pass.
func reconcileFilerMetadata(ctx context.Context, runtime Runtime, files ProviderFiles) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	root := "/etc/baseharbor/ha-probes/" + hex.EncodeToString(nonce[:])
	defer removeFilerMetadataMarkers(runtime, files, root)
	if err := createFilerMetadataMarkers(ctx, runtime, files, root); err != nil {
		return fmt.Errorf("prepare SeaweedFS metadata replication proof: %w", err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	missing, err := waitFilerMetadataMarkers(probeCtx, runtime, files, root)
	cancel()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("SeaweedFS metadata replication did not converge: %w", ctx.Err())
	}
	if err := rejoinFilerMetadataMembers(ctx, runtime, files, root, missing); err != nil {
		return err
	}
	if _, err := waitFilerMetadataMarkers(ctx, runtime, files, root); err != nil {
		return fmt.Errorf("SeaweedFS metadata replication after member rejoin: %w", err)
	}
	return nil
}

func filerShell(ctx context.Context, runtime Runtime, files ProviderFiles, member, commands string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return runtime.ExecProjectInput(callCtx, files.Project, files.Compose, files.Env,
		[]byte(commands), member, "weed", "shell", "-filer="+member+":8888")
}

func createFilerMetadataMarkers(ctx context.Context, runtime Runtime, files ProviderFiles, root string) error {
	for _, member := range filerMembers {
		for {
			if _, err := filerShell(ctx, runtime, files, member, "fs.mkdir "+root+"/"+member+"\n"); err == nil {
				break
			}
			if err := waitFilerProbeTick(ctx); err != nil {
				return fmt.Errorf("create metadata marker on %s: %w", member, err)
			}
		}
	}
	return nil
}

func missingFilerMetadataMarkers(ctx context.Context, runtime Runtime, files ProviderFiles, root string) []string {
	var commands strings.Builder
	for _, source := range filerMembers {
		fmt.Fprintf(&commands, "fs.meta.cat %s/%s\n", root, source)
	}
	var missing []string
	for _, reader := range filerMembers {
		if !filerMetadataReaderReady(ctx, runtime, files, reader, commands.String()) {
			missing = append(missing, reader)
		}
	}
	return missing
}

func filerMetadataReaderReady(ctx context.Context, runtime Runtime, files ProviderFiles, reader, commands string) bool {
	out, err := filerShell(ctx, runtime, files, reader, commands)
	if err != nil {
		return false
	}
	for _, source := range filerMembers {
		if !strings.Contains(out, `"name": "`+source+`"`) {
			return false
		}
	}
	return true
}

func waitFilerMetadataMarkers(ctx context.Context, runtime Runtime, files ProviderFiles, root string) ([]string, error) {
	for {
		missing := missingFilerMetadataMarkers(ctx, runtime, files, root)
		if len(missing) == 0 {
			return nil, nil
		}
		if err := waitFilerProbeTick(ctx); err != nil {
			return missing, fmt.Errorf("metadata readers %s are incomplete: %w", strings.Join(missing, ", "), err)
		}
	}
}

func waitFilerProbeTick(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func rejoinFilerMetadataMembers(ctx context.Context, runtime Runtime, files ProviderFiles, root string, members []string) error {
	reconciler, ok := runtime.(providerPKIRuntimeReconciler)
	if !ok {
		return fmt.Errorf("SeaweedFS metadata readers %s require runtime member reconciliation", strings.Join(members, ", "))
	}
	environment, err := readProviderValues(files.Env)
	if err != nil {
		return err
	}
	for _, member := range members {
		if err := reconciler.UpProjectFilesSelectedForceRecreateNoBuild(ctx, files.Project, files.Dir, environment, []string{member}, files.Compose); err != nil {
			return fmt.Errorf("rejoin SeaweedFS metadata reader %s: %w", member, err)
		}
		// Restore all incoming replication paths before changing another HA
		// member. Its own store is durable across a container recreation.
		var commands strings.Builder
		for _, source := range filerMembers {
			fmt.Fprintf(&commands, "fs.meta.cat %s/%s\n", root, source)
		}
		for {
			if filerMetadataReaderReady(ctx, runtime, files, member, commands.String()) {
				break
			}
			if err := waitFilerProbeTick(ctx); err != nil {
				return fmt.Errorf("restore SeaweedFS metadata reader %s: %w", member, err)
			}
		}
	}
	return nil
}

func removeFilerMetadataMarkers(runtime Runtime, files ProviderFiles, root string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, member := range filerMembers {
		_, _ = filerShell(ctx, runtime, files, member, "fs.rm -rf "+root+"\n")
	}
}
