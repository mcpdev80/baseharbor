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

func TestRabbitMQHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_RABBITMQ_HA_ACCEPTANCE") != "1" {
		t.Skip("RabbitMQ HA acceptance requires BASEHARBOR_RABBITMQ_HA_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "rabbitmq-ha-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "rabbitmq-ha-ci",
		Environment:   "dev",
		HA:            true,
		Services: Services{
			MessagingQueue:        true,
			MessagingPubSub:       true,
			MessagingStream:       true,
			MessagingManagementUI: true,
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
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 20*time.Second)
			if logs, logErr := runtime.LogsProject(diagnosticCtx, files.Project, files.Compose, files.Env); logErr == nil {
				t.Logf("RabbitMQ HA project logs:\n%s", logs)
			} else {
				t.Logf("RabbitMQ HA project log capture failed: %v", logErr)
			}
			diagnosticCancel()
		}
		if t.Failed() && os.Getenv("BASEHARBOR_RABBITMQ_HA_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy RabbitMQ HA runtime: %v", err)
		}
	}()

	if err := ReconcileRabbitMQCredentials(ctx, provideroperation.New(runtime, files.Project, files.Compose, files.Env), m, files); err != nil {
		t.Fatal(err)
	}
	op := provideroperation.New(runtime, files.Project, files.Compose, files.Env)
	waitRabbitMQHAReady(t, ctx, op, m, files)

	beforeRotation, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	oldUser := beforeRotation[rabbitmqRuntimeKey(defaultServiceInstance, "USER")]
	oldPassword := beforeRotation[rabbitmqRuntimeKey(defaultServiceInstance, "PASSWORD")]
	oldAdminPassword := beforeRotation[rabbitmqRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD")]
	if err := RotateRabbitMQCredential(ctx, runtime, m, files, defaultServiceInstance); err != nil {
		t.Fatalf("rotate RabbitMQ HA credentials: %v", err)
	}
	afterRotation, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	if afterRotation[rabbitmqRuntimeKey(defaultServiceInstance, "USER")] == oldUser ||
		afterRotation[rabbitmqRuntimeKey(defaultServiceInstance, "PASSWORD")] == oldPassword ||
		afterRotation[rabbitmqRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD")] == oldAdminPassword {
		t.Fatal("RabbitMQ credential rotation did not replace application/admin credentials")
	}
	waitRabbitMQHAReady(t, ctx, op, m, files)

	environment, err := RuntimeEnvironment(files)
	if err != nil {
		t.Fatal(err)
	}
	failedMember := rabbitmqMemberServiceName(defaultServiceInstance, 1)
	if err := runtime.StopProjectFilesSelected(
		ctx,
		files.Project,
		files.Dir,
		environment,
		[]string{failedMember},
		files.Compose,
	); err != nil {
		t.Fatalf("stop RabbitMQ HA member %s: %v", failedMember, err)
	}

	failoverDeadline := time.Now().Add(45 * time.Second)
	for {
		if err := VerifyRabbitMQRuntime(ctx, m, files); err == nil {
			if uiErr := VerifyApplicationManagementUIs(ctx, m, files); uiErr == nil {
				break
			}
		}
		if time.Now().After(failoverDeadline) {
			t.Fatal("RabbitMQ stable AMQPS/management endpoints did not survive one member failure")
		}
		time.Sleep(time.Second)
	}

	if err := runtime.UpProjectFilesSelected(
		ctx,
		files.Project,
		files.Dir,
		environment,
		[]string{failedMember},
		files.Compose,
	); err != nil {
		t.Fatalf("restart RabbitMQ HA member %s: %v", failedMember, err)
	}
	waitRabbitMQHAReady(t, ctx, provideroperation.New(runtime, files.Project, files.Compose, files.Env), m, files)
}

func waitRabbitMQHAReady(t *testing.T, ctx context.Context, runtime rabbitMQHAProbeRuntime, m Manifest, files RuntimeFiles) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for {
		semanticErr := VerifyRabbitMQRuntime(ctx, m, files)
		clusterErr := VerifyRabbitMQHACluster(ctx, runtime, m, files)
		uiErr := VerifyApplicationManagementUIs(ctx, m, files)
		if semanticErr == nil && clusterErr == nil && uiErr == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("RabbitMQ HA did not become ready: semantic=%v cluster=%v ui=%v", semanticErr, clusterErr, uiErr)
		}
		time.Sleep(time.Second)
	}
}
