package application

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRabbitMQTLSGatewayAndServiceBinding(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "events",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:          true,
			MessagingQueueInstances: map[string]ServiceInstance{"jobs": {}},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	compose, err := RuntimeComposeYAMLForProject(m, "bh-events")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rabbitmq-jobs:", "rabbitmq-jobs-access:", "RABBITMQ_JOBS_HOST_PORT"} {
		if !strings.Contains(compose, want) {
			t.Fatalf("RabbitMQ TLS compose missing %q:\n%s", want, compose)
		}
	}

	root := t.TempDir()
	ca := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(ca, []byte("-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := RuntimeFiles{
		Dir:      root,
		Env:      filepath.Join(root, "runtime.env"),
		Bindings: filepath.Join(root, "bindings"),
	}
	values := map[string]string{
		rabbitmqRuntimeKey("jobs", "USER"):      "baseharbor",
		rabbitmqRuntimeKey("jobs", "PASSWORD"):  "super-secret",
		rabbitmqRuntimeKey("jobs", "HOST_PORT"): "35672",
		rabbitmqTLSCAKey("jobs"):                ca,
	}
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		t.Fatal(err)
	}
	contract, err := EnsureRuntimeContract(m, files)
	if err != nil {
		t.Fatal(err)
	}
	bindingDir := filepath.Join(contract.BindingsDir, "rabbitmq", "jobs")
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(bindingDir, name))
		if err != nil {
			t.Fatalf("read binding %s: %v", name, err)
		}
		return strings.TrimSpace(string(data))
	}
	if got := read("type"); got != "rabbitmq" {
		t.Fatalf("type = %q", got)
	}
	if got := read("username"); got != "baseharbor" {
		t.Fatalf("username = %q", got)
	}
	if got := read("password"); got != "super-secret" {
		t.Fatalf("password mismatch")
	}
	if got := read("certificates"); !strings.Contains(got, "BEGIN CERTIFICATE") {
		t.Fatalf("certificates missing CA material")
	}
	uri := read("uri")
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "amqps" || u.Host != "127.0.0.1:35672" {
		t.Fatalf("RabbitMQ binding URI = %q", uri)
	}
	if u.User == nil {
		t.Fatalf("RabbitMQ binding URI has no credentials")
	}
	if err := VerifyWorkloadServiceBindings(m, files); err != nil {
		t.Fatal(err)
	}
}
