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

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestFreshRestoreStableConsentAndCanonicalIdentityRecovery(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "")
	configureTestTarget(t)
	root := t.TempDir()
	t.Chdir(root)
	archived := application.New("fresh-recovery", "dev", true, false, false)
	source := []byte(archived.AuthoredYAML())
	sourcePath := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(sourcePath, source, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := machineNoninteractiveContext(context.Background())
	data := applicationRestoreData{manifest: archived, archiveRevision: strings.Repeat("a", 64)}
	var planID string
	var prior deployment.DeploymentIdentity
	var latest resolvedApplication
	for attempt := 0; attempt < 3; attempt++ {
		r, err := resolveRestoreTarget(ctx, application.Store{}, archived)
		if err != nil {
			t.Fatal(err)
		}
		if attempt > 0 && r.DeploymentIdentity == prior {
			t.Fatal("test did not exercise distinct unallocated deployment proposals")
		}
		prior = r.DeploymentIdentity
		p, err := buildApplicationRestorePlan(ctx, r, data)
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			planID = p.Scope.PlanID
		} else if p.Scope.PlanID != planID {
			t.Fatal("unchanged recovery proposals invalidated consent")
		}
		if p.Scope.Attention != "Achtung" || p.Scope.ConfirmationsRequired != 2 || !p.Scope.Destructive {
			t.Fatal("missing current recovery consequences")
		}
		for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: planID, First: true}, {PlanID: planID, First: true, Second: true}} {
			if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, ack), r, data, io.Discard); err == nil {
				t.Fatal("missing acknowledgement or data consent admitted")
			}
		}
		if err := materializeRestoreIdentity(ctx, r, data); err == nil {
			t.Fatal("identity mutation bypassed verified recovery authority")
		}
		if records, err := deployment.ListDeployments(r.Target.Name); err != nil || len(records) != 0 {
			t.Fatal("preflight allocated deployment owner", err)
		}
		if _, err := os.Stat(filepath.Join(root, ".baseharbor", "apps")); !os.IsNotExist(err) {
			t.Fatal("preflight allocated source owner")
		}
		latest = r
	}
	ack := &application.ProviderChangeConsent{PlanID: planID, First: true, Second: true, DeleteData: true}
	authorized, unlock, err := prepareAuthorizedApplicationRestore(withProviderChangeConsent(ctx, ack), latest, data, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := materializeRestoreIdentity(authorized, latest, data); err != nil {
		t.Fatal(err)
	}
	if err := materializeRestoreIdentity(authorized, latest, data); err == nil {
		t.Fatal("changed pending owner reused prior pre-mutation authority")
	}
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	recovered, _, err := owner.Load(archived.Name)
	if err != nil || recovered.ApplicationID != archived.ApplicationID {
		t.Fatal("archive identity was replaced", err)
	}
	retry, err := resolveRestoreTarget(ctx, application.Store{}, archived)
	if err != nil || retry.DeploymentIdentity != latest.DeploymentIdentity {
		t.Fatal("interrupted recovery lost pending deployment identity", err)
	}
	if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, ack), retry, data, io.Discard); err == nil {
		t.Fatal("new pending owner reused pre-mutation consent")
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(source, after) {
		t.Fatal("recovery rewrote authored intent")
	}
}

func TestRestoreShowsConsequencesBeforeRefusingUnavailableReplacement(t *testing.T) {
	r := corelessResolvedFixture(t)
	r.FromRepository = false
	id, err := deployment.NewDeploymentIdentity(r.Target.Name, r.Manifest.ApplicationID, r.Manifest.Name, r.Manifest.Environment)
	if err != nil {
		t.Fatal(err)
	}
	r.DeploymentIdentity = id
	r.DeploymentStateRoot, err = deployment.DeploymentRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	r.Manifest.Services.Identity = true
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "shared")
	if err := application.ReconcileReferenceProviderRegistryAt(r.TargetStateRoot, r.Manifest); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(r.TargetStateRoot, "provider-registry.json")
	original, err := os.ReadFile(registry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), "application")
	data := applicationRestoreData{manifest: r.Manifest, archiveRevision: strings.Repeat("a", 64)}
	ctx := machineNoninteractiveContext(t.Context())
	plan, err := buildApplicationRestorePlan(ctx, r, data)
	if err != nil {
		t.Fatal("technical admission hid recovery consequences", err)
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: plan.Scope.PlanID, First: true, Second: true, DeleteData: true}} {
		_, unlock, err := prepareAuthorizedApplicationRestore(withProviderChangeConsent(ctx, ack), r, data, io.Discard)
		if unlock != nil {
			unlock()
		}
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatal("restore bypassed plan or admission", err)
		}
		if ack == nil && failure.Code != machine.ErrorApprovalRequired {
			t.Fatal("adapter refusal hid acknowledgements", err)
		}
		if ack != nil && failure.Code != machine.ErrorUnsupported {
			t.Fatal("unavailable replacement executed after consent", err)
		}
	}
	after, err := os.ReadFile(registry)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("denied restore changed original bindings")
	}
	if _, err := os.Stat(r.DeploymentStateRoot); !os.IsNotExist(err) {
		t.Fatal("denied restore allocated deployment owner")
	}
}

func TestFreshRestoreRejectsNewSourceAndOwnerBeforeMutation(t *testing.T) {
	for _, change := range []string{"source", "foreign-source-owner", "concurrent-deployment", "archive", "placement", "workload"} {
		t.Run(change, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "")
			configureTestTarget(t)
			root := t.TempDir()
			t.Chdir(root)
			m := application.New("fresh-drift", "dev", true, false, false)
			sourcePath := filepath.Join(root, application.RepositoryManifestName)
			if err := os.WriteFile(sourcePath, []byte(m.AuthoredYAML()), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := machineNoninteractiveContext(context.Background())
			r, err := resolveRestoreTarget(ctx, application.Store{}, m)
			if err != nil {
				t.Fatal(err)
			}
			data := applicationRestoreData{manifest: m, archiveRevision: strings.Repeat("a", 64)}
			p, err := buildApplicationRestorePlan(ctx, r, data)
			if err != nil {
				t.Fatal(err)
			}
			approved := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
			verified, unlock, err := prepareAuthorizedApplicationRestore(withProviderChangeConsent(ctx, approved), r, data, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()
			switch change {
			case "source":
				changed := m
				changed.Services.Cache = true
				if err := os.WriteFile(sourcePath, []byte(changed.AuthoredYAML()), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign-source-owner":
				changed := m
				changed.ApplicationID = application.MustNewApplicationID()
				if _, err := (application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}).Create(changed); err != nil {
					t.Fatal(err)
				}
			case "concurrent-deployment":
				concurrent, err := resolveRestoreTarget(ctx, application.Store{}, m)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := recordPendingDeployment(ctx, concurrent); err != nil {
					t.Fatal(err)
				}
			case "placement":
				t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "application")
				current, err := resolveRestoreTarget(ctx, application.Store{}, m)
				if err != nil {
					t.Fatal(err)
				}
				next, err := buildApplicationRestorePlan(ctx, current, data)
				if err != nil {
					t.Fatal(err)
				}
				if next.Scope.PlanID == p.Scope.PlanID {
					t.Fatal("fresh placement change did not invalidate plan")
				}
			case "workload":
				if err := os.WriteFile(filepath.Join(root, "application.txt"), []byte("changed repository payload"), 0600); err != nil {
					t.Fatal(err)
				}
			case "archive":
				data.archiveRevision = strings.Repeat("b", 64)
			}
			ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true, DeleteData: true}
			if err := authorizeApplicationRestore(withProviderChangeConsent(ctx, ack), r, data, io.Discard); err == nil {
				t.Fatal("changed recovery authority reused consent")
			}
			if err := materializeRestoreIdentity(verified, r, data); err == nil {
				t.Fatal("post-approval recovery drift reached identity mutation")
			}
			if _, err := os.Stat(r.DeploymentStateRoot); !os.IsNotExist(err) {
				t.Fatal("denied restore materialized proposed deployment")
			}
		})
	}
}
