package main

import (
    "context"
    "errors"
    "fmt"
    "io"
    "path/filepath"

    "github.com/mcpdev80/baseharbor/internal/coreupdate"
    bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// streamPatroniDCSSnapshot exports an etcd v3 point-in-time snapshot through
// the owned Core etcd member. It never streams the live etcd data directory:
// only etcdctl's consistent snapshot artifact is returned.
func streamPatroniDCSSnapshot(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, writer io.Writer) error {
    if rt == nil || !files.HA || files.Project == "" || files.Compose == "" || files.Env == "" || writer == nil {
        return errors.New("Patroni DCS snapshot requires owned HA runtime and output stream")
    }
    environment, err := bhruntime.RuntimeEnvironment(files)
    if err != nil { return err }
    const script = "set -eu\numask 077\nsnapshot=/etcd-data/.baseharbor-upgrade-snapshot.db\nif [ -e \"$snapshot\" ]; then exit 17; fi\ntrap 'rm -f \"$snapshot\"' EXIT\nETCDCTL_API=3 etcdctl --endpoints=http://127.0.0.1:2379 snapshot save \"$snapshot\" >/dev/null\ncat \"$snapshot\""
    if err := rt.RunProjectFilesEnv(ctx, files.Project, filepath.Dir(files.Compose), environment,
        nil, writer, io.Discard, []string{files.Compose},
        "exec", "-T", "postgres-etcd-1", "sh", "-ec", script); err != nil {
        return fmt.Errorf("native Patroni etcd DCS snapshot failed: %w", err)
    }
    return nil
}

// captureOwnedPatroniDCSSnapshot is independently checksum-protected, so a
// basebackup is never misrepresented as including Patroni's etcd state.
func captureOwnedPatroniDCSSnapshot(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files, directory string) error {
    snapshot := coreupdate.StreamRecoveryPoint{Directory:directory, Name:"core-patroni-dcs"}
    return snapshot.Capture(ctx, func(ctx context.Context, w io.Writer) error {
        return streamPatroniDCSSnapshot(ctx, rt, files, w)
    })
}
