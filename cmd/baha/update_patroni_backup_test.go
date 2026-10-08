package main

import (
    "bytes"
    "context"
    "errors"
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"

    bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type streamingBasebackupRuntime struct {
    bhruntime.RuntimeProvider
    argv []string
    passwordInput string
    fail bool
}
func (r *streamingBasebackupRuntime) RunProjectFilesEnv(_ context.Context, _, _ string, _ map[string]string, stdin io.Reader, stdout, _ io.Writer, _ []string, args ...string) error {
    input, err := io.ReadAll(stdin)
    if err != nil { return err }
    r.passwordInput = string(input)
    r.argv = append([]string(nil), args...)
    if r.fail { return errors.New("replication stream interrupted") }
    _, err = io.Copy(stdout, strings.NewReader(strings.Repeat("TAR-physical-postgres-WAL", 2048)))
    return err
}
func TestStreamPatroniBasebackupNoSecretsInArgv(t *testing.T) {
    dir := t.TempDir()
    env := filepath.Join(dir, "runtime.env")
    if err := os.WriteFile(env, []byte("BASEHARBOR_POSTGRES_USER=test\n"),0600);err!=nil{t.Fatal(err)}
    files := bhruntime.Files{HA:true,Project:"baseharbor-owned",Compose:filepath.Join(dir,"compose.yaml"),Env:env}
    rt := &streamingBasebackupRuntime{}
    var archive bytes.Buffer
    const secret = "secret-must-never-be-command-argument"
    if err := streamPatroniBasebackup(context.Background(),rt,files,"postgres-member-1","replication",secret,&archive);err!=nil{t.Fatal(err)}
    if archive.Len() == 0 || rt.passwordInput != secret+"\n" {t.Fatal("provider-native stream or protected credential input missing")}
    if strings.Contains(strings.Join(rt.argv," "),secret) {t.Fatal("replication password leaked through command argv")}
    if !strings.Contains(strings.Join(rt.argv," "), "pg_basebackup") || !strings.Contains(strings.Join(rt.argv," "),"-X fetch") {t.Fatal("missing WAL-inclusive native backup")}
    if err := streamPatroniBasebackup(context.Background(),rt,files,"foreign-member","replication",secret,&archive);err==nil {t.Fatal("foreign PostgreSQL source accepted")}
    rt.fail=true
    if err := streamPatroniBasebackup(context.Background(),rt,files,"postgres-member-2","replication",secret,&archive);err==nil{t.Fatal("interrupted WAL stream returned success")}
}
