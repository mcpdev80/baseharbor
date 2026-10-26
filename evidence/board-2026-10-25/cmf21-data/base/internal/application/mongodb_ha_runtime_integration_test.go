package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestMongoDBHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_MONGODB_HA_ACCEPTANCE") != "1" {
		t.Skip("MongoDB HA acceptance requires BASEHARBOR_MONGODB_HA_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "mongodb-ha-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "mongodb-ha-ci",
		Environment:   "dev",
		HA:            true,
		Services: Services{
			DocumentDatabase:             true,
			DocumentDatabaseManagementUI: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	issuer := serviceissuer.New(t)
	files, err := EnsureRuntime(ctx, issuer, store, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() {
			t.Logf("MongoDB HA diagnostics:\n%s", runtime.DiagnosticsProject(context.Background(), files.Project, files.Compose, files.Env))
			if os.Getenv("BASEHARBOR_MONGODB_HA_KEEP_ON_FAILURE") == "1" {
				return
			}
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy MongoDB HA runtime: %v", err)
		}
	}()

	op := provideroperation.New(runtime, files.Project, files.Compose, files.Env)
	if err := ReconcileMongoDBHA(ctx, op, m, files); err != nil {
		t.Fatal(err)
	}
	waitMongoDBHAReady(t, ctx, op, m, files)

	beforeRotation, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	oldAppUser := beforeRotation[mongodbRuntimeKey(defaultServiceInstance, "USER")]
	oldAppPassword := beforeRotation[mongodbRuntimeKey(defaultServiceInstance, "PASSWORD")]
	oldAdminUser := beforeRotation[mongodbRuntimeKey(defaultServiceInstance, "ADMIN_USER")]
	oldAdminPassword := beforeRotation[mongodbRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD")]
	if err := RotateMongoDBCredential(ctx, runtime, m, files, defaultServiceInstance); err != nil {
		t.Fatalf("rotate MongoDB HA credentials: %v", err)
	}
	afterRotation, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if afterRotation[mongodbRuntimeKey(defaultServiceInstance, "USER")] == oldAppUser ||
		afterRotation[mongodbRuntimeKey(defaultServiceInstance, "PASSWORD")] == oldAppPassword ||
		afterRotation[mongodbRuntimeKey(defaultServiceInstance, "ADMIN_USER")] == oldAdminUser ||
		afterRotation[mongodbRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD")] == oldAdminPassword {
		t.Fatal("MongoDB credential rotation did not replace application/admin credentials")
	}
	waitMongoDBHAReady(t, ctx, op, m, files)

	oldProviderCA, err := os.ReadFile(afterRotation[mongodbTLSCAKey(defaultServiceInstance)])
	if err != nil {
		t.Fatal(err)
	}
	oldUICA, err := os.ReadFile(filepath.Join(files.Dir, "providers", "management-ui", "mongodb", defaultServiceInstance, "pki", "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if err := RotateManagedProviderPKI(ctx, runtime, serviceissuer.New(t), m, files, ManagedProviderPKIMongoDB, defaultServiceInstance); err != nil {
		t.Fatalf("rotate MongoDB HA PKI: %v", err)
	}
	rotatedValues, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	assertOldCARootRejected(t, oldProviderCA, rotatedValues[mongodbMemberHostPortKey(defaultServiceInstance, 0)])
	assertOldCARootRejected(t, oldUICA, rotatedValues[mongodbUIHostPortKey(defaultServiceInstance)])
	waitMongoDBHAReady(t, ctx, op, m, files)

	failedMember, err := MongoDBHAPrimary(ctx, op, m, defaultServiceInstance)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("stop MongoDB HA primary %s: %v", failedMember, err)
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		newPrimary, primaryErr := MongoDBHAPrimary(ctx, op, m, defaultServiceInstance)
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		if primaryErr == nil && newPrimary != failedMember && uiErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MongoDB HA did not preserve election/UI continuity after primary failure: old=%s new=%s primaryErr=%v ui=%v", failedMember, newPrimary, primaryErr, uiErr)
		}
		time.Sleep(time.Second)
	}

	if err := runtime.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{failedMember}, files.Compose); err != nil {
		t.Fatalf("restart MongoDB HA member %s: %v", failedMember, err)
	}
	waitMongoDBHAReady(t, ctx, op, m, files)
}

func waitMongoDBHAReady(t *testing.T, ctx context.Context, op mongoDBHAProbeRuntime, m Manifest, files RuntimeFiles) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		haErr := VerifyMongoDBHACluster(ctx, op, m, files)
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		if haErr == nil && uiErr == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("MongoDB HA did not become ready: ha=%v ui=%v", haErr, uiErr)
		}
		time.Sleep(time.Second)
	}
}
