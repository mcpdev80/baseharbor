package main

import (
    "context"
    "errors"
    "os"
    "strings"
    "testing"

    "github.com/mcpdev80/baseharbor/internal/coreinstallation"
    "github.com/mcpdev80/baseharbor/internal/deployment"
    "github.com/mcpdev80/baseharbor/internal/machine"
)

func TestRemoteTargetSelectionDoesNotRequireOrBootstrapLocalCore(t *testing.T) {
    _, _, node := remoteApplicationCoreFixtureEmpty(t)
    root, err := targetRuntimeStateRoot(node)
    if err != nil { t.Fatal(err) }

    err = bindExplicitSelectionToConnection(context.Background(), node.Name)
    var typed *machine.Error
    if !errors.As(err, &typed) || typed.Code != machine.ErrorAuthenticationFailed {
        t.Fatalf("remote selection without verified binding must fail closed: %v", err)
    }
    if !strings.Contains(err.Error(), "remote Core management connection") {
        t.Fatalf("incorrect remote connection diagnostic: %v", err)
    }
    if strings.Contains(err.Error(), "Install a BaseHarbor Core") {
        t.Fatalf("developer-only remote client was incorrectly told to install local Core: %v", err)
    }
    if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("remote selection created local Core installation: %v", err)
    }
    if _, err := deployment.TargetStateRoot(node.Name); err != nil {
        t.Fatal(err)
    }
}
