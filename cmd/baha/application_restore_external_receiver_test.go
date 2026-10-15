package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

func TestRestoreBindsCurrentExternalReceiverAndAuthorization(t *testing.T) {
	for _, change := range []string{"receiver", "authorization", "anonymous", "fresh-receiver", "fresh-authorization"} {
		t.Run(change, func(t *testing.T) {
			r := externalReceiverPlanFixture(t)
			if strings.HasPrefix(change, "fresh-") {
				files := application.RuntimeFilesFor(r.Store, r.Manifest)
				for _, path := range []string{files.Env, files.ApplicationEnv} {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			}
			var err error
			r.DeploymentIdentity, err = deployment.NewDeploymentIdentity(r.Target.Name, r.Manifest.ApplicationID, r.Manifest.Name, r.Manifest.Environment)
			if err != nil {
				t.Fatal(err)
			}
			r.DeploymentStateRoot, err = deployment.DeploymentRoot(r.DeploymentIdentity)
			if err != nil {
				t.Fatal(err)
			}
			ctx := machineNoninteractiveContext(t.Context())
			data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
			planned, err := buildApplicationRestorePlan(ctx, r, data)
			if err != nil {
				t.Fatal(err)
			}
			switch strings.TrimPrefix(change, "fresh-") {
			case "receiver":
				t.Setenv(application.OTLPEndpointEnv, "https://receiver-after.example.test")
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "authorization":
				t.Setenv(telemetry.ExternalHeadersEnv, "Authorization=Bearer fixture-sensitive-after")
			case "anonymous":
				t.Setenv(telemetry.ExternalHeadersEnv, "")
			}
			current, err := buildApplicationRestorePlan(ctx, r, data)
			if err != nil {
				t.Fatal(err)
			}
			if current.Scope.PlanID == planned.Scope.PlanID {
				t.Fatal("changed external receiver authorization did not invalidate restore plan")
			}
			ack := &application.ProviderChangeConsent{PlanID: planned.Scope.PlanID, First: true, Second: true, DeleteData: true}
			if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, ack), r, data, io.Discard); err == nil {
				t.Fatal("changed external delivery reused restore consent")
			}
			raw, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"fixture-sensitive-before", "fixture-sensitive-after", "Authorization=Bearer"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("restore consequence projection exposed authorization")
				}
			}
		})
	}
}
