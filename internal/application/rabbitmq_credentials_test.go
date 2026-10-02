package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rabbitCredentialRuntimeFake struct {
	input   []byte
	service string
	args    []string
}

func (f *rabbitCredentialRuntimeFake) RunSensitive(_ context.Context, service string, input []byte, args ...string) (string, error) {
	f.input = append([]byte(nil), input...)
	f.service = service
	f.args = append([]string(nil), args...)
	return "", nil
}

func TestReconcileRabbitMQCredentialsUsesStdinAndSeparateClasses(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "prod",
		Services: Services{
			MessagingQueue:        true,
			MessagingManagementUI: true,
		},
	}
	root := t.TempDir()
	files := RuntimeFiles{
		Dir:     root,
		Env:     filepath.Join(root, "runtime.env"),
		Compose: filepath.Join(root, "compose.yaml"),
		Project: "bh-events",
	}
	values := map[string]string{
		rabbitmqRuntimeKey(defaultServiceInstance, "USER"):               "app-user",
		rabbitmqRuntimeKey(defaultServiceInstance, "PASSWORD"):           "app-secret",
		rabbitmqRuntimeKey(defaultServiceInstance, "BOOTSTRAP_USER"):     "internal-user",
		rabbitmqRuntimeKey(defaultServiceInstance, "BOOTSTRAP_PASSWORD"): "internal-secret",
		rabbitmqRuntimeKey(defaultServiceInstance, "ADMIN_USER"):         "admin-user",
		rabbitmqRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD"):     "admin-secret",
		rabbitmqRuntimeKey(defaultServiceInstance, "HOST_PORT"):          "35672",
	}
	if err := os.WriteFile(files.Env, []byte(runtimeEnvContent(m, values)), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &rabbitCredentialRuntimeFake{}
	if err := ReconcileRabbitMQCredentials(context.Background(), fake, m, files); err != nil {
		t.Fatal(err)
	}
	gotInput := string(fake.input)
	for _, want := range []string{"app-user\n", "app-secret\n", "admin-user\n", "admin-secret\n"} {
		if !strings.Contains(gotInput, want) {
			t.Fatalf("stdin missing %q: %q", want, gotInput)
		}
	}
	joinedArgs := strings.Join(fake.args, " ")
	for _, secret := range []string{"app-secret", "admin-secret", "internal-secret"} {
		if strings.Contains(joinedArgs, secret) {
			t.Fatalf("secret %q leaked into runtime command args: %q", secret, joinedArgs)
		}
	}
	if fake.service != "rabbitmq" {
		t.Fatalf("credential reconciliation service = %q", fake.service)
	}
}

func TestRabbitMQRuntimeEnvKeepsCredentialClassesDistinct(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "prod",
		Services: Services{
			MessagingQueue:        true,
			MessagingManagementUI: true,
		},
	}
	env, err := newRuntimeEnv(m)
	if err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnvBytes([]byte(env))
	if err != nil {
		t.Fatal(err)
	}
	appUser := values[rabbitmqRuntimeKey(defaultServiceInstance, "USER")]
	adminUser := values[rabbitmqRuntimeKey(defaultServiceInstance, "ADMIN_USER")]
	bootstrapUser := values[rabbitmqRuntimeKey(defaultServiceInstance, "BOOTSTRAP_USER")]
	if appUser == "" || adminUser == "" || bootstrapUser == "" {
		t.Fatalf("RabbitMQ credential classes incomplete: app=%q admin=%q bootstrap=%q", appUser, adminUser, bootstrapUser)
	}
	if appUser == bootstrapUser || adminUser == bootstrapUser {
		t.Fatalf("RabbitMQ Class C bootstrap identity leaked into app/management identity")
	}
}
