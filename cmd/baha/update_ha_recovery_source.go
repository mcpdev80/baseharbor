package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type coreHARecoverySource struct {
	Evidence                                                                                             coreupdate.DCSRecoveryEvidence
	ComposeSHA, EnvSHA, PostgresSHA, PostgresImage, EtcdImage, Leader, PostgresOwner, PostgresDataSubdir string
}

func postgresRecoveryDataSubdir(root, data string) (string, error) {
	if root != "/home/postgres/pgdata/pgroot" {
		return "", errors.New("unverified PostgreSQL volume root")
	}
	for _, subdir := range []string{"data", "pgdata"} {
		if data == root+"/"+subdir {
			return subdir, nil
		}
	}
	return "", errors.New("PostgreSQL data directory is outside the supported owned volume layout")
}

func privateRecoveryFile(path string) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 1<<20 {
		return nil, errors.New("unsafe HA recovery metadata")
	}
	return os.ReadFile(path)
}

func recoveryDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func postgresRecoveryDigest(journal string) (string, error) {
	p := coreupdate.StreamRecoveryPoint{Directory: filepath.Join(journal, "patroni-recovery"), Name: "core-spilo-basebackup"}
	f, err := p.OpenVerified()
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadCoreHARecoverySource(journal, installation, target, release string) (coreHARecoverySource, []byte, error) {
	var source coreHARecoverySource
	data, err := privateRecoveryFile(filepath.Join(journal, "ha-recovery-source.json"))
	if err != nil {
		return source, nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&source); err != nil {
		return source, nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return source, nil, errors.New("trailing HA recovery metadata")
	}
	if source.Evidence.Installation != installation || source.Evidence.Target != target || source.Evidence.Release != release ||
		source.Evidence.Cluster == "" || len(source.Evidence.SHA256) != 64 || !strings.HasPrefix(source.Leader, "postgres-member-") ||
		!strings.Contains(source.PostgresImage, "@sha256:") || !strings.Contains(source.EtcdImage, "@sha256:") {
		return source, nil, errors.New("HA recovery point belongs to a different Core, Target or release")
	}
	if source.Leader != "postgres-member-1" && source.Leader != "postgres-member-2" && source.Leader != "postgres-member-3" {
		return source, nil, errors.New("foreign recovery leader")
	}
	if source.PostgresDataSubdir != "data" && source.PostgresDataSubdir != "pgdata" {
		return source, nil, errors.New("unverified PostgreSQL recovery data directory")
	}
	owner := strings.Split(source.PostgresOwner, ":")
	if len(owner) != 2 {
		return source, nil, errors.New("invalid PostgreSQL recovery owner")
	}
	for _, part := range owner {
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 || id > 65000 {
			return source, nil, errors.New("invalid PostgreSQL recovery owner")
		}
	}
	for _, image := range []string{source.PostgresImage, source.EtcdImage} {
		parts := strings.Split(image, "@sha256:")
		if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 64 {
			return source, nil, errors.New("invalid pinned HA recovery image")
		}
		if _, err := hex.DecodeString(parts[1]); err != nil {
			return source, nil, err
		}
	}
	compose, err := privateRecoveryFile(filepath.Join(journal, "ha-recovery-compose.yaml"))
	if err != nil {
		return source, nil, err
	}
	digest, err := postgresRecoveryDigest(journal)
	if err != nil {
		return source, nil, err
	}
	if recoveryDigest(compose) != source.ComposeSHA || digest != source.PostgresSHA {
		return source, nil, errors.New("HA recovery source checksum mismatch")
	}
	return source, compose, nil
}

func captureCoreHARecoverySource(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, journal, leader string, ev coreupdate.DCSRecoveryEvidence) error {
	path := filepath.Join(journal, "ha-recovery-source.json")
	if _, err := os.Lstat(path); err == nil {
		s, _, err := loadCoreHARecoverySource(journal, ev.Installation, ev.Target, ev.Release)
		if err == nil && s.Evidence != ev {
			return errors.New("HA recovery evidence changed")
		}
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	compose, err := privateRecoveryFile(files.Compose)
	if err != nil {
		return err
	}
	environment, err := privateRecoveryFile(files.Env)
	if err != nil {
		return err
	}
	pg, err := rt.ProjectServiceImageIdentity(ctx, files.Project, leader)
	if err != nil {
		return err
	}
	pgImage, err := runtimePinnedImage(pg)
	if err != nil {
		return err
	}
	etcd, err := rt.ProjectServiceImageIdentity(ctx, files.Project, "postgres-etcd-1")
	if err != nil {
		return err
	}
	etcdImage, err := runtimePinnedImage(etcd)
	if err != nil {
		return err
	}
	var ids []string
	for _, flag := range []string{"-u", "-g"} {
		output, err := rt.ExecProject(ctx, files.Project, files.Compose, files.Env, leader, "id", flag, "postgres")
		if err != nil {
			return err
		}
		id, err := strconv.Atoi(strings.TrimSpace(output))
		if err != nil || id <= 0 || id > 65000 {
			return errors.New("invalid non-root PostgreSQL recovery owner")
		}
		ids = append(ids, strconv.Itoa(id))
	}
	layout, err := rt.ExecProject(ctx, files.Project, files.Compose, files.Env, leader, "python3", "-c", "import os; print(os.environ.get('PGROOT','')); print(os.environ.get('PGDATA',''))")
	if err != nil {
		return err
	}
	paths := strings.Split(strings.TrimSpace(layout), "\n")
	if len(paths) != 2 {
		return errors.New("unverified PostgreSQL data layout")
	}
	subdir, err := postgresRecoveryDataSubdir(paths[0], paths[1])
	if err != nil {
		return err
	}
	digest, err := postgresRecoveryDigest(journal)
	if err != nil {
		return err
	}
	s := coreHARecoverySource{Evidence: ev, ComposeSHA: recoveryDigest(compose), EnvSHA: recoveryDigest(environment), PostgresSHA: digest, PostgresImage: pgImage, EtcdImage: etcdImage, Leader: leader, PostgresOwner: strings.Join(ids, ":"), PostgresDataSubdir: subdir}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := writeRecoveryCompose(filepath.Join(journal, "ha-recovery-compose.yaml"), compose); err != nil {
		return err
	}
	if err := writeRecoveryCompose(path, data); err != nil {
		return fmt.Errorf("persist HA recovery source: %w", err)
	}
	return nil
}
