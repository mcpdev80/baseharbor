package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

type nativeHACutover struct {
	runtime              bhruntime.RuntimeProvider
	files                bhruntime.Files
	source               coreHARecoverySource
	journal, recoveryDir string
	projected            []byte
	volumes              map[string]string
	tools                runtimeEtcdTools
}

func projectHARecoveryCompose(original []byte, source coreHARecoverySource, project, recoveryDir string, identity etcdRecoveryIdentity) ([]byte, map[string]string, error) {
	var model map[string]any
	if err := yaml.Unmarshal(original, &model); err != nil {
		return nil, nil, err
	}
	services, ok := model["services"].(map[string]any)
	if !ok {
		return nil, nil, errors.New("missing HA recovery services")
	}
	for name := range services {
		if (strings.HasPrefix(name, "postgres-member-") || strings.HasPrefix(name, "postgres-etcd-")) && name != coreEtcdRecoveryService && !haRecoveryMember(name) {
			return nil, nil, fmt.Errorf("unrecognized additional HA member %s", name)
		}
	}
	volumes, ok := model["volumes"].(map[string]any)
	if !ok {
		return nil, nil, errors.New("missing managed HA volumes")
	}
	newVolumes := map[string]string{}
	for i := 1; i <= 3; i++ {
		pg := fmt.Sprintf("postgres-member-%d", i)
		dcs := fmt.Sprintf("postgres-etcd-%d", i)
		for _, name := range []string{pg, dcs} {
			svc, ok := services[name].(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("missing owned HA member %s", name)
			}
			mounts, ok := svc["volumes"].([]any)
			if !ok {
				return nil, nil, errors.New("missing HA member data mount")
			}
			target := "/etcd-data"
			replacement := filepath.Join(recoveryDir, dcs) + ":" + target
			if name == pg {
				target = "/home/postgres/pgdata/pgroot"
				logical := fmt.Sprintf("postgres-recovered-%s-%d", recoveryDigest([]byte(source.PostgresSHA + source.Evidence.SHA256))[:16], i)
				actual := project + "_" + logical
				newVolumes[pg] = actual
				volumes[logical] = map[string]any{"name": actual}
				replacement = logical + ":" + target
				svc["image"] = source.PostgresImage
			} else {
				svc["image"] = source.EtcdImage
				svc["user"] = identity.User
				if identity.UserNS != "" {
					svc["userns_mode"] = identity.UserNS
				} else {
					delete(svc, "userns_mode")
				}
			}
			found := 0
			for index, mount := range mounts {
				value, ok := mount.(string)
				if !ok {
					return nil, nil, errors.New("unsupported HA recovery mount syntax")
				}
				parts := strings.Split(value, ":")
				if len(parts) < 2 || parts[1] != target {
					continue
				}
				found++
				mounts[index] = replacement
			}
			if found != 1 {
				return nil, nil, fmt.Errorf("ambiguous data mount for %s", name)
			}
		}
	}
	data, err := yaml.Marshal(model)
	return data, newVolumes, err
}

func replacePrivateHACompose(path string, data []byte) error {
	if _, err := privateRecoveryFile(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("HA Compose parent is not private")
	}
	f, err := os.CreateTemp(dir, ".ha-cutover-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (o *nativeHACutover) environment() (map[string]string, error) {
	return bhruntime.RuntimeEnvironment(o.files)
}
func (o *nativeHACutover) stop(ctx context.Context, services []string) error {
	env, err := o.environment()
	if err != nil {
		return err
	}
	return o.runtime.StopProjectFilesSelected(ctx, o.files.Project, filepath.Dir(o.files.Compose), env, services, o.files.Compose)
}
func (o *nativeHACutover) up(ctx context.Context, services []string, force bool) error {
	env, err := o.environment()
	if err != nil {
		return err
	}
	if force {
		return o.runtime.UpProjectFilesSelectedForceRecreateNoBuild(ctx, o.files.Project, filepath.Dir(o.files.Compose), env, services, o.files.Compose)
	}
	return o.runtime.UpProjectFilesSelectedNoBuildProgress(ctx, o.files.Project, filepath.Dir(o.files.Compose), env, services, nil, o.files.Compose)
}
func haRecoveryMember(service string) bool {
	for i := 1; i <= 3; i++ {
		if service == fmt.Sprintf("postgres-member-%d", i) || service == fmt.Sprintf("postgres-etcd-%d", i) {
			return true
		}
	}
	return false
}

func (o *nativeHACutover) FenceOldDCS(ctx context.Context) error {
	containers, err := o.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}
	var ids []string
	seen := map[string]bool{}
	for _, c := range containers {
		if c.Project == o.files.Project && (strings.HasPrefix(c.Service, "postgres-member-") || strings.HasPrefix(c.Service, "postgres-etcd-")) && c.Service != coreEtcdRecoveryService && !haRecoveryMember(c.Service) {
			return errors.New("unrecognized runtime HA member; refusing incomplete fencing")
		}
		if c.Project != o.files.Project || !haRecoveryMember(c.Service) {
			continue
		}
		if c.ID == "" || seen[c.Service] {
			return errors.New("ambiguous owned HA fence inventory")
		}
		seen[c.Service] = true
		ids = append(ids, c.ID)
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	if err := writeRecoveryCompose(filepath.Join(o.journal, "ha-fenced-containers.json"), data); err != nil {
		return err
	}
	if err := o.stop(ctx, o.files.PostgresMembers()); err != nil {
		return err
	}
	containers, err = o.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.Project == o.files.Project && strings.HasPrefix(c.Service, "postgres-member-") && c.Running {
			return errors.New("PostgreSQL writers still active before DCS fencing")
		}
	}
	if err := o.stop(ctx, o.tools.Members); err != nil {
		return err
	}
	return o.VerifyFenced(ctx)
}

func (o *nativeHACutover) VerifyFenced(ctx context.Context) error {
	data, err := privateRecoveryFile(filepath.Join(o.journal, "ha-fenced-containers.json"))
	if err != nil {
		return err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return err
	}
	old := map[string]bool{}
	for _, id := range ids {
		old[id] = true
	}
	containers, err := o.runtime.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.Project != o.files.Project || !haRecoveryMember(c.Service) || !c.Running {
			continue
		}
		if old[c.ID] {
			return fmt.Errorf("old HA member %s is not fenced", c.Service)
		}
		// After activation new containers may run under the same service names.
		// Prove their actual mounts; changed container IDs alone are insufficient.
		cli, ok := o.runtime.(interface {
			DirectOutput(context.Context, ...string) (string, error)
		})
		if !ok {
			return errors.New("runtime cannot verify active recovery mounts")
		}
		output, err := cli.DirectOutput(ctx, "inspect", c.ID, "--format", "{{json .Mounts}}")
		if err != nil {
			return err
		}
		var mounts []struct{ Type, Name, Source, Destination string }
		if err := json.Unmarshal([]byte(output), &mounts); err != nil {
			return err
		}
		found := false
		for _, m := range mounts {
			if volume, ok := o.volumes[c.Service]; ok {
				if m.Destination == "/home/postgres/pgdata/pgroot" && m.Type == "volume" && m.Name == volume {
					found = true
				}
			} else if m.Destination == "/etcd-data" && m.Type == "bind" && filepath.Clean(m.Source) == filepath.Join(o.recoveryDir, c.Service) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("HA member %s still uses old or foreign data", c.Service)
		}
	}
	return nil
}

func (o *nativeHACutover) ActivateIsolated(ctx context.Context, ev coreupdate.DCSRecoveryEvidence) error {
	if ev != o.source.Evidence {
		return coreupdate.ErrDCSInvalidEvidence
	}
	if err := o.VerifyFenced(ctx); err != nil {
		return err
	}
	seedPath := filepath.Join(o.journal, "ha-volumes-prepared")
	receipt := []byte(recoveryDigest(o.projected))
	if existing, err := privateRecoveryFile(seedPath); err == nil {
		if string(existing) != string(receipt) {
			return errors.New("replacement volumes belong to another recovery point")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		for _, member := range o.files.PostgresMembers() {
			if err := o.runtime.EnsureOwnedVolume(ctx, o.files.Project, o.volumes[member]); err != nil {
				return err
			}
		}
		seeder, ok := o.runtime.(interface {
			SeedOwnedVolume(context.Context, string, string, string, string, string, io.Reader) error
		})
		if !ok {
			return errors.New("native streaming volume recovery unavailable")
		}
		point := coreupdate.StreamRecoveryPoint{Directory: filepath.Join(o.journal, "patroni-recovery"), Name: "core-spilo-basebackup"}
		f, err := point.OpenVerified()
		if err != nil {
			return err
		}
		err = seeder.SeedOwnedVolume(ctx, o.files.Project, o.volumes[o.source.Leader], o.source.PostgresImage, o.source.PostgresDataSubdir, o.source.PostgresOwner, f)
		f.Close()
		if err != nil {
			return err
		}
		if err := writeRecoveryCompose(seedPath, receipt); err != nil {
			return err
		}
	} else {
		return err
	}
	for _, volume := range o.volumes {
		owned, err := o.runtime.InspectProjectResource(ctx, o.files.Project, bhruntime.ProjectResource{Kind: "volume", Name: volume})
		if err != nil || !owned {
			return errors.New("replacement volume ownership lost")
		}
	}
	current, err := privateRecoveryFile(o.files.Compose)
	if err != nil {
		return err
	}
	prior, err := privateRecoveryFile(filepath.Join(o.journal, "ha-before-cutover.yaml"))
	if err != nil {
		return err
	}
	if string(current) != string(prior) && string(current) != string(o.projected) {
		return errors.New("managed Compose drift during HA cutover")
	}
	if err := replacePrivateHACompose(o.files.Compose, o.projected); err != nil {
		return err
	}
	return o.up(ctx, o.tools.Members, true)
}

func (o *nativeHACutover) VerifyNewQuorum(ctx context.Context, ev coreupdate.DCSRecoveryEvidence) error {
	if ev != o.source.Evidence {
		return coreupdate.ErrDCSInvalidEvidence
	}
	meta, err := (etcdbackup.Store{Directory: filepath.Join(o.journal, "dcs-snapshot"), Identity: etcdbackup.Identity{Core: ev.Installation, Target: ev.Target, Cluster: ev.Cluster}}).VerifySnapshot(ctx)
	if err != nil {
		return err
	}
	tools := o.tools
	tools.ExpectedCluster = ""
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		info, err := tools.attest(bounded)
		if err == nil {
			if info.ClusterID == ev.Cluster || info.Revision < meta.Info.Revision || info.Version != meta.Info.Version {
				return errors.New("active restored DCS identity/revision/version mismatch")
			}
			return nil
		}
		select {
		case <-bounded.Done():
			return fmt.Errorf("restored DCS quorum not ready: %w", err)
		case <-time.After(time.Second):
		}
	}
}

// Report only fixed filesystem paths, states and error categories; no SQL,
// credentials, Patroni configuration or arbitrary server messages are emitted.
const haRecoveryPrimaryDiagnostic = `import os, glob, json, urllib.request
root=os.environ['PGROOT']
data=os.environ['PGDATA']
try:
 d=json.load(urllib.request.urlopen('http://127.0.0.1:8008/patroni',timeout=3))
 print('patroni',json.dumps({k:d.get(k) for k in ('state','role','timeline')}))
except Exception: print('patroni-status-unavailable')
for name in ('','pgdata','pgdata/PG_VERSION','pgdata/postgresql.conf','pgdata/postgresql.base.conf','pgdata/pg_hba.conf','pgdata/pg_ident.conf','pgdata/backup_label','pgdata/recovery.signal','pgdata/standby.signal','pg_log'):
 p=os.path.join(data,name[7:]) if name.startswith('pgdata/') else (data if name=='pgdata' else os.path.join(root,name))
 try:
  st=os.stat(p); print('path',name,oct(st.st_mode & 0o777),st.st_uid,st.st_gid)
 except OSError: print('missing',name)
patterns=('Permission denied','No such file or directory','could not locate a valid checkpoint record','database system identifier differs','recovery ended before configured recovery target','PANIC','FATAL','postgresql.conf','postgresql.base.conf','pg_hba.conf','pg_ident.conf','pg_wal','backup_label')
for p in glob.glob(root+'/pg_log/*'):
 try:
  with open(p,'rb') as f:
   f.seek(max(0,os.fstat(f.fileno()).st_size-131072)); data=f.read().decode('utf-8','replace')
  print('log-categories',json.dumps([v for v in patterns if v in data]))
 except OSError: print('log-unreadable')
`

func (o *nativeHACutover) VerifyPatroniDCS(ctx context.Context) error {
	if err := o.VerifyFenced(ctx); err != nil {
		return err
	}
	if err := o.up(ctx, []string{o.source.Leader}, false); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	const primary = "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/primary',timeout=3).close()"
	for {
		_, err := o.runtime.ExecProject(bounded, o.files.Project, o.files.Compose, o.files.Env, o.source.Leader, "python3", "-c", primary)
		if err == nil {
			break
		}
		select {
		case <-bounded.Done():
			diagnosticCtx, diagnosticCancel := context.WithTimeout(ctx, 15*time.Second)
			diagnostic, _ := o.runtime.ExecProject(diagnosticCtx, o.files.Project, o.files.Compose, o.files.Env, o.source.Leader, "python3", "-c", haRecoveryPrimaryDiagnostic)
			diagnosticCancel()
			return fmt.Errorf("restored PostgreSQL primary not ready: %w; diagnostics: %s", err, diagnostic)
		case <-time.After(time.Second):
		}
	}
	if err := o.up(ctx, o.files.PostgresMembers(), false); err != nil {
		return err
	}
	if err := coreupdate.WaitForPatroniQuorum(bounded, &patroniCoreRollingOps{runtime: o.runtime, files: o.files}, o.source.Leader, 0, 90*time.Second); err != nil {
		return err
	}
	credentials, err := bhruntime.LoadControlPlaneCredentials(o.files)
	if err != nil {
		return err
	}
	if err := probeControlPlanePostgresCredential(ctx, o.runtime, o.files, credentials.PostgresUser, credentials.PostgresPassword, "postgres"); err != nil {
		return err
	}
	// Check actual recovery mounts after native startup too.
	return o.VerifyFenced(ctx)
}

func (o *nativeHACutover) CommitCutover(ctx context.Context, ev coreupdate.DCSRecoveryEvidence) error {
	if ev != o.source.Evidence {
		return coreupdate.ErrDCSInvalidEvidence
	}
	if err := o.VerifyFenced(ctx); err != nil {
		return err
	}
	current, err := privateRecoveryFile(o.files.Compose)
	if err != nil {
		return err
	}
	if string(current) != string(o.projected) {
		return errors.New("HA recovery Compose changed before commit")
	}
	// Original volumes and the exact prior Compose are retained, never deleted.
	return writeRecoveryCompose(filepath.Join(o.journal, "ha-cutover-committed"), []byte(recoveryDigest(o.projected)+" "+ev.SHA256))
}

// Only the three SQL image pins may differ from the captured recovery manifest.
// Other provider/configuration changes need reconciliation before fencing.
func validateHARecoveryComposeDrift(original, current []byte) error {
	var documents [2]map[string]any
	for i, raw := range [][]byte{original, current} {
		if err := yaml.Unmarshal(raw, &documents[i]); err != nil {
			return err
		}
		services, ok := documents[i]["services"].(map[string]any)
		if !ok {
			return errors.New("recovery manifest has no service mapping")
		}
		for _, name := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
			service, ok := services[name].(map[string]any)
			if !ok {
				return errors.New("recovery manifest lacks a required SQL member")
			}
			delete(service, "image")
		}
	}
	if !reflect.DeepEqual(documents[0], documents[1]) {
		return errors.New("Core configuration outside the SQL image pins changed since recovery capture; reconcile before fencing")
	}
	return nil
}

func recoverOwnedCoreHA(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, journal, installation, target, release string) error {
	if rt == nil || !files.HA {
		return errors.New("owned HA runtime recovery required")
	}
	source, original, err := loadCoreHARecoverySource(journal, installation, target, release)
	if err != nil {
		return err
	}
	environment, err := privateRecoveryFile(files.Env)
	if err != nil {
		return err
	}
	if source.EnvSHA != recoveryDigest(environment) {
		return errors.New("Core credentials/environment changed since the recovery point; reconcile before recovery")
	}
	if err := prepareOwnedPatroniPhysicalRestore(ctx, journal); err != nil {
		return err
	}
	identity, err := inspectEtcdRecoveryIdentity(ctx, rt)
	if err != nil {
		return err
	}
	current, err := privateRecoveryFile(files.Compose)
	if err != nil {
		return err
	}
	// The prior manifest is saved once; activated/committed replay must retain it.
	priorPath := filepath.Join(journal, "ha-before-cutover.yaml")
	if _, err := os.Lstat(priorPath); errors.Is(err, os.ErrNotExist) {
		if err := validateHARecoveryComposeDrift(original, current); err != nil {
			return err
		}
		if err := writeRecoveryCompose(priorPath, current); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	prior, err := privateRecoveryFile(priorPath)
	if err != nil {
		return err
	}
	if err := validateHARecoveryComposeDrift(original, prior); err != nil {
		return err
	}
	members := []string{"postgres-etcd-1", "postgres-etcd-2", "postgres-etcd-3"}
	recoveryDir := filepath.Join(journal, "dcs-isolated-restore")
	projected, volumes, err := projectHARecoveryCompose(prior, source, files.Project, recoveryDir, identity)
	if err != nil {
		return err
	}
	if recoveryDigest(current) != recoveryDigest(prior) && recoveryDigest(current) != recoveryDigest(projected) {
		return errors.New("active Core manifest changed outside the recovery transaction")
	}
	tools := runtimeEtcdTools{Runtime: rt, Files: files, Service: coreEtcdRecoveryService, Endpoints: []string{"https://postgres-etcd-1:2379", "https://postgres-etcd-2:2379", "https://postgres-etcd-3:2379"}, ExpectedCluster: source.Evidence.Cluster, ScratchDir: filepath.Join(journal, "dcs-scratch"), ContainerCA: "/run/baseharbor/etcd/ca.pem", ContainerCert: "/run/baseharbor/etcd/client.pem", ContainerKey: "/run/baseharbor/etcd/client-key.pem", Members: members, InitialCluster: "postgres-etcd-1=https://postgres-etcd-1:2380,postgres-etcd-2=https://postgres-etcd-2:2380,postgres-etcd-3=https://postgres-etcd-3:2380", Identity: identity}
	bridge := &coreupdate.EtcdDCSBridge{Store: etcdbackup.Store{Directory: filepath.Join(journal, "dcs-snapshot"), Identity: etcdbackup.Identity{Core: installation, Target: target, Cluster: source.Evidence.Cluster}}, Restorer: tools, RecoveryDirectory: recoveryDir, Release: release}
	bridge.VerifyRecoveredCluster = func(ctx context.Context, _ etcdbackup.Identity, snapshot etcdbackup.SnapshotInfo) error {
		return verifyRuntimeRecoveredEtcdCluster(ctx, rt, files, tools, recoveryDir, source.EtcdImage, "baseharbor-control-postgres", snapshot)
	}
	ops := &nativeHACutover{runtime: rt, files: files, source: source, journal: journal, recoveryDir: recoveryDir, projected: projected, volumes: volumes, tools: tools}
	bridge.LiveRestore = func(ctx context.Context, ev coreupdate.DCSRecoveryEvidence) error {
		return coreupdate.RunVerifiedDCSCutover(ctx, bridge, ev, installation, target, ev.Cluster, release, ops, coreupdate.DCSCutoverJournal{Path: filepath.Join(journal, "ha-cutover.journal")})
	}
	return bridge.Restore(ctx, source.Evidence)
}
