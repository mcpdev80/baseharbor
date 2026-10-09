package main

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestProviderConfigurationRestoreReplaysAndRejectsTampering(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.env")
	original := []byte("SECRET=original\n")
	if err := os.WriteFile(config, original, 0600); err != nil {
		t.Fatal(err)
	}
	spec := providerSQLBackupSpec{Directory: filepath.Join(dir, "backup"), Name: "openbao", ConfigPaths: []string{config}}
	point := coreupdate.StreamRecoveryPoint{Directory: spec.Directory, Name: "openbao-config"}
	err := point.Capture(context.Background(), func(ctx context.Context, w io.Writer) error {
		tw := tar.NewWriter(w)
		if err := tw.WriteHeader(&tar.Header{Name: "0", Mode: 0600, Size: int64(len(original)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(original); err != nil {
			return err
		}
		return tw.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("SECRET=changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := spec.restoreConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("configuration not recovered: %q", got)
	}
	if err := spec.restoreConfiguration(context.Background()); err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if err := os.WriteFile(config, []byte("SECRET=changed-again\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(config, 0666); err != nil {
		t.Fatal(err)
	}
	if err := spec.restoreConfiguration(context.Background()); err == nil {
		t.Fatal("unsafe destination accepted")
	}
	got, err = os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "changed-again") {
		t.Fatal("unsafe destination unexpectedly mutated")
	}
	if err := os.Chmod(config, 0600); err != nil {
		t.Fatal(err)
	}
	checksum := filepath.Join(spec.Directory, "openbao-config.backup.sha256")
	if err := os.WriteFile(checksum, []byte(strings.Repeat("0", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := spec.restoreConfiguration(context.Background()); err == nil {
		t.Fatal("tampered archive accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestProviderBackupPairRejectsPartialCaptureAndBindsBothStreams(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "core.env")
	if err := os.WriteFile(config, []byte("SECRET=original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := providerSQLBackupSpec{Directory: dir, Name: "openbao", Provider: "openbao", InstallationID: "core-A", Transaction: "0.4.24", Project: "owned", Compose: filepath.Join(dir, "compose.yaml"), Env: config, Host: "postgres", Database: "openbao", ConfigPaths: []string{config}}
	if complete, err := spec.recoveryPairComplete(); err != nil || complete {
		t.Fatalf("expected fresh pair: %v %t", err, complete)
	}
	if err := os.WriteFile(filepath.Join(dir, "openbao-sql.backup"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := spec.recoveryPairComplete(); err == nil {
		t.Fatal("partial SQL/config pair accepted")
	}
	if err := os.Remove(filepath.Join(dir, "openbao-sql.backup")); err != nil {
		t.Fatal(err)
	}
	if err := spec.streamPoint().Capture(context.Background(), func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "SQL archive bytes")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := spec.captureConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if complete, err := spec.recoveryPairComplete(); err != nil || !complete {
		t.Fatalf("completed pair rejected: %v %t", err, complete)
	}
	one, err := spec.artifactBinding("2.7.0")
	if err != nil {
		t.Fatal(err)
	}
	two, err := spec.artifactBinding("2.7.1")
	if err != nil || one == two {
		t.Fatal("provider original version not bound to SQL/config pair")
	}
	spec.Project = "foreign"
	foreign, err := spec.artifactBinding("2.7.0")
	if err != nil || one == foreign {
		t.Fatal("foreign installation project reused recovery binding")
	}
	spec.Project = "owned"
	spec.InstallationID = "core-B"
	otherInstall, err := spec.artifactBinding("2.7.0")
	if err != nil || one == otherInstall { t.Fatal("cross-installation SQL/config snapshot admitted") }
	spec.InstallationID = "core-A"
	spec.Transaction = "0.4.25"
	otherTxn, err := spec.artifactBinding("2.7.0")
	if err != nil || one == otherTxn { t.Fatal("cross-transaction SQL/config snapshot admitted") }
}

type sqlRestoreReplayRuntime struct {
	bhruntime.RuntimeProvider
	failSQL bool
	restores int
}
func (r *sqlRestoreReplayRuntime) RunProjectFilesEnv(_ context.Context, _, _ string, _ map[string]string, stdin io.Reader, _, _ io.Writer, _ []string, args ...string) error {
	if len(args) > 0 && args[0] == "run" {
		_, _ = io.Copy(io.Discard, stdin)
		for _, arg := range args {
			if arg == "sh" {
				r.restores++
				if r.failSQL { return errors.New("injected pg_restore failure") }
			}
		}
		return nil
	}
	return errors.New("unexpected restore runtime operation")
}
func TestProviderSQLRestoreFailureDoesNotChangeConfigurationAndResumeRestoresBoth(t *testing.T) {
	dir:=t.TempDir()
	if err:=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
	env:=filepath.Join(dir,"owned.env")
	if err:=os.WriteFile(env,[]byte("CORE_OWNED=yes\n"),0600);err!=nil{t.Fatal(err)}
	rt:=&sqlRestoreReplayRuntime{failSQL:true}
	spec:=providerSQLBackupSpec{Runtime:rt,Project:"owned-core",Compose:filepath.Join(dir,"compose.yaml"),Env:env,Client:"pgclient",Host:"postgres",CAFile:"/ca.pem",User:"owner",Password:"secret",Database:"openbao",Directory:filepath.Join(dir,"backup"),Name:"openbao",Provider:providerupgrade.ProviderOpenBao,InstallationID:"core-A",Transaction:"0.4.24",ConfigPaths:[]string{env}}
	if err:=spec.streamPoint().Capture(context.Background(),func(_ context.Context,w io.Writer)error{_,e:=io.WriteString(w,"mock custom SQL archive");return e});err!=nil{t.Fatal(err)}
	if err:=spec.captureConfiguration(context.Background());err!=nil{t.Fatal(err)}
	binding,err:=spec.artifactBinding("2.7.0");if err!=nil{t.Fatal(err)}
	ref:=providerupgrade.BackupRef{Provider:spec.Provider,ID:spec.Name,Version:"2.7.0",Verified:true,Metadata:map[string]string{"database_verified":"true","configuration_verified":"true","format":"pg_dump-custom-v1","binding":binding}}
	if err:=os.WriteFile(env,[]byte("CORE_OWNED=changed\n"),0600);err!=nil{t.Fatal(err)}
	if err:=spec.restore(context.Background(),ref);err==nil{t.Fatal("failed SQL restore returned success")}
	data,err:=os.ReadFile(env);if err!=nil{t.Fatal(err)}
	if string(data)!="CORE_OWNED=changed\n"{t.Fatal("configuration changed despite failed SQL restore")}
	rt.failSQL=false
	if err:=spec.restore(context.Background(),ref);err!=nil{t.Fatalf("idempotent paired recovery rejected: %v",err)}
	data,err=os.ReadFile(env);if err!=nil{t.Fatal(err)}
	if string(data)!="CORE_OWNED=yes\n"{t.Fatalf("configuration recovery missing after SQL replay: %q",data)}
	if rt.restores!=2 {t.Fatalf("expected two SQL restore attempts, got %d",rt.restores)}
}
