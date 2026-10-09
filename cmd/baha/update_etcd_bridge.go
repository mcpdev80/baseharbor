package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func runtimePinnedImage(identity bhruntime.ImageIdentity) (string, error) {
	ref := strings.TrimSpace(identity.Reference)
	digest := strings.TrimSpace(identity.Digest)
	if ref == "" || digest == "" {
		return "", errors.New("runtime image identity is not digest pinned")
	}
	if at := strings.LastIndex(digest, "@sha256:"); at >= 0 {
		digest = digest[at+1:]
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		return "", errors.New("runtime etcd image requires a full sha256 digest")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:")); err != nil {
		return "", fmt.Errorf("invalid runtime etcd image digest: %w", err)
	}
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	return ref + "@" + digest, nil
}

func buildCoreEtcdRecoveryBridge(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, installation, target, release, journalDir string) (*coreupdate.EtcdDCSBridge, error) {
	if rt == nil || !files.HA || installation == "" || target == "" || release == "" || journalDir == "" {
		return nil, coreupdate.ErrDCSUnsupported
	}
	members := []string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"}
	endpoints := make([]string, 0, len(members))
	clusterParts := make([]string, 0, len(members))
	for _, member := range members {
		endpoints = append(endpoints, "https://"+member+":2379")
		clusterParts = append(clusterParts, member+"=https://"+member+":2380")
	}
	tools := runtimeEtcdTools{
		Runtime: rt, Files: files, Service: coreEtcdRecoveryService, Endpoints: endpoints,
		ScratchDir:  filepath.Join(journalDir, "dcs-scratch"),
		ContainerCA: "/run/baseharbor/etcd/ca.pem", ContainerCert: "/run/baseharbor/etcd/client.pem", ContainerKey: "/run/baseharbor/etcd/client-key.pem",
		Members: members, InitialCluster: strings.Join(clusterParts, ","),
	}
	info, err := tools.attest(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover authenticated Core etcd identity: %w", err)
	}
	tools.ExpectedCluster = info.ClusterID
	imageIdentity, err := rt.ProjectServiceImageIdentity(ctx, files.Project, members[0])
	if err != nil {
		return nil, fmt.Errorf("inspect Core etcd image identity: %w", err)
	}
	image, err := runtimePinnedImage(imageIdentity)
	if err != nil {
		return nil, err
	}
	store := etcdbackup.Store{
		Directory: filepath.Join(journalDir, "dcs-snapshot"),
		Identity:  etcdbackup.Identity{Core: installation, Target: target, Cluster: info.ClusterID},
		Client:    tools,
	}
	recoveryDir := filepath.Join(journalDir, "dcs-isolated-restore")
	bridge := &coreupdate.EtcdDCSBridge{
		Store: store, Restorer: tools, RecoveryDirectory: recoveryDir, Release: release,
	}
	bridge.VerifyRecoveredCluster = func(verifyCtx context.Context, identity etcdbackup.Identity, snapshot etcdbackup.SnapshotInfo) error {
		if identity != store.Identity {
			return coreupdate.ErrDCSInvalidEvidence
		}
		return verifyRuntimeRecoveredEtcdCluster(verifyCtx, rt, files, tools, recoveryDir, image, "baseharbor-control-postgres", snapshot)
	}
	return bridge, nil
}
