package main

import (
    "context"
    "io"
    "strings"
    "testing"

    "github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestControlPlaneOnlyRemoteTargetFailsBeforeLocalBootstrap(t *testing.T) {
    configureTestTargetEmpty(t)
    cfg, err := deployment.LoadConfig()
    if err != nil { t.Fatal(err) }
    cfg.Access["connector"] = deployment.AccessDefinition{Provider: "baseharbor-node-connector", Reference: "node-a"}
    cfg.Targets["remote"] = deployment.TargetDefinition{
        TenantID: "11111111-1111-4111-8111-111111111111",
        Runtime: deployment.RuntimeDefinition{Provider: "docker"},
        Access: deployment.TargetAccess{Reference: "connector"},
    }
    if err := cfg.Save(); err != nil { t.Fatal(err) }
    t.Setenv("BASEHARBOR_TARGET", "remote")
    t.Chdir(t.TempDir())
    for _, run := range []struct {
        name string
        call func() error
    }{
        {"user-command", func() error { return runtimeUpCommandWithInputResolver(context.Background(), []string{"--control-plane-only", "--yes"}, io.Discard, io.Discard) }},
        {"runtime-entry", func() error { return runtimeUpCommand(context.Background(), []string{"--control-plane-only", "--yes"}, io.Discard, io.Discard) }},
    } {
        t.Run(run.name, func(t *testing.T) {
            err := run.call()
            if err == nil || !strings.Contains(err.Error(), "remote control-plane bootstrap is not supported") {
                t.Fatalf("remote Core bootstrap did not fail safely: %v", err)
            }
            if _, err := deployment.TargetStateRoot("remote"); err != nil { t.Fatal(err) }
        })
    }
}
