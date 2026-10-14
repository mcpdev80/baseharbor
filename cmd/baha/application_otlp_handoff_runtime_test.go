package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

// Exercise the common current-plan authority and actual mounted consumer,
// rather than equating a projected environment or host-side probe with handoff.
func TestManagedRegisteredOTLPNativeConsumerHandoff(t *testing.T) {
	if os.Getenv("BASEHARBOR_OTLP_HANDOFF_ACCEPTANCE") != "1" {
		t.Skip("requires isolated native Docker/rootless Podman OTLP handoff acceptance")
	}
	ctx, cancel := context.WithTimeout(machineNoninteractiveContext(t.Context()), 12*time.Minute)
	defer cancel()
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv(application.OTLPEndpointEnv, "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
	t.Setenv(telemetry.ExternalHeadersEnv, "")
	if err := os.Unsetenv(telemetry.ExternalHeadersEnv); err != nil {
		t.Fatal(err)
	}
	target := configureTestTarget(t)
	if os.Getenv("BASEHARBOR_TEST_RUNTIME") == "podman" {
		cfg, err := deployment.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		definition := cfg.Targets[target.Name]
		definition.Runtime.Provider = "podman"
		cfg.Targets[target.Name] = definition
		if err := cfg.Save(); err != nil {
			t.Fatal(err)
		}
		target, err = cfg.ResolveTarget("", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	assertCorelessHost(t, ctx, runtime, target)
	receiver := startNativeOTLPReceiver(t, ctx, runtime)
	installNativeOTLPFixtureDNS(t, receiver.host)
	receiver.waitReady(t, ctx)
	t.Chdir(t.TempDir())
	workload := strings.Replace(corelessHTTPWorkload, "    healthcheck:", "    networks: [receiver]\n    healthcheck:", 1)
	workload += fmt.Sprintf("\nnetworks:\n  receiver:\n    external: true\n    name: %q\n", receiver.network)
	for name, data := range map[string]string{
		"compose.yaml":    workload,
		"baseharbor.yaml": "version: 1\napp:\n  name: otlp-handoff\n  environment: dev\nworkload:\n  components: [api]\ntelemetry:\n  otlp:\n    export: [traces]\n",
	} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "init", "--input", "tls_mode=local", "--yes", "--json"}, &output, &output); err != nil {
		t.Fatal("initialize native OTLP source", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(machineNoninteractiveContext(context.Background()), 3*time.Minute)
		defer stop()
		if err := approvedNativeFixtureCleanup(t, cleanup, &bytes.Buffer{}); err != nil {
			t.Errorf("owned OTLP fixture cleanup: %v", err)
		}
	})
	managed := applySelectiveIdentityIntent(t, ctx)
	files := application.RuntimeFilesFor(managed.Store, managed.Manifest)
	payload := hex.EncodeToString(telemetry.VerificationTracePayload(managed.Manifest))
	runNativeOTLPConsumer(t, ctx, payload, "managed-before")
	provider, err := telemetry.ExistingProviderFilesAt(managed.TargetStateRoot, target.Name)
	if err != nil {
		t.Fatal(err)
	}
	managedMaterial := nativeOTLPMaterialSnapshot(t, filepath.Join(provider.Dir, "service-access", "pki"))
	if err := application.RegisterExternalProviderAt(managed.TargetStateRoot, receiver.registration); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), receiver.registration.ID)
	before := nativeOTLPMaterialSnapshot(t, files.Dir)
	plan, err := buildResolvedApplicationPlan(ctx, managed)
	if err != nil || plan.ProviderPlan.Impact == nil {
		t.Fatal("native replacement lacks concrete consequences", err)
	}
	impact := plan.ProviderPlan.Impact
	if impact.Attention != "Achtung" || impact.ConfirmationsRequired != 2 {
		t.Fatal("native replacement omits Achtung/two current confirmations")
	}
	for _, consent := range []*application.ProviderChangeConsent{nil, {PlanID: impact.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}} {
		var rejected bytes.Buffer
		if err := runWithIO(withProviderChangeConsent(ctx, consent), []string{"app", "apply", "--skip-memory-preflight"}, &rejected, &rejected); err == nil {
			t.Fatal("native replacement bypassed current confirmations")
		}
		assertNativeOTLPMaterialSnapshot(t, before)
	}
	external := applySelectiveIdentityIntent(t, ctx)
	if external.Manifest.ApplicationID != managed.Manifest.ApplicationID {
		t.Fatal("handoff replaced application identity")
	}
	runNativeOTLPConsumer(t, ctx, payload, "registered-workload")
	receiver.assertWorkload(t, ctx, runtime, "registered-workload", payload)
	receiver.assertMaterials(t)
	assertNativeOTLPMaterialSnapshot(t, managedMaterial)
	if services, err := runtime.RunningServicesProject(ctx, provider.Project, provider.Compose, provider.Env); err != nil || len(services) != 0 {
		t.Fatal("old managed Collector was not retired after verified replacement", err)
	}
	applySelectiveIdentityIntent(t, ctx)
	runNativeOTLPConsumer(t, ctx, payload, "registered-repeat")
	receiver.assertWorkload(t, ctx, runtime, "registered-repeat", payload)
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOTLP), "")
	returned := applySelectiveIdentityIntent(t, ctx)
	if returned.Manifest.ApplicationID != managed.Manifest.ApplicationID {
		t.Fatal("return replaced application identity")
	}
	runNativeOTLPConsumer(t, ctx, payload, "managed-after")
	receiver.assertMaterials(t)
	assertNativeOTLPMaterialSnapshot(t, managedMaterial)
	if err := destroySelectiveApplicationWithConsent(ctx); err != nil {
		t.Fatal("default owned application retirement", err)
	}
	receiver.assertMaterials(t)
	if services, err := runtime.RunningServicesProject(ctx, receiver.project, receiver.compose, receiver.env); err != nil || len(services) != 1 {
		t.Fatal("application retirement mutated foreign receiver", err)
	}
	t.Log("actual managed -> registered -> managed native Protobuf consumers, current confirmations, original protected material and foreign receiver preservation PASS")
}

func runNativeOTLPConsumer(t *testing.T, ctx context.Context, payload, actor string) {
	t.Helper()
	const script = `import os,ssl,sys,urllib.request
context=ssl.create_default_context(cafile=os.environ['OTEL_EXPORTER_OTLP_CERTIFICATE'])
cert=os.environ.get('OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE','')
if cert: context.load_cert_chain(cert,os.environ['OTEL_EXPORTER_OTLP_CLIENT_KEY'])
headers={'Content-Type':'application/x-protobuf','X-Fixture-Workload':sys.argv[2]}
for pair in os.environ.get('OTEL_EXPORTER_OTLP_HEADERS','').split(','):
 if '=' in pair:
  key,value=pair.split('=',1);headers[key]=value
url=os.environ['OTEL_EXPORTER_OTLP_ENDPOINT'].rstrip('/')+'/v1/traces'
request=urllib.request.Request(url,data=bytes.fromhex(sys.argv[1]),headers=headers,method='POST')
with urllib.request.urlopen(request,context=context,timeout=15) as response:
 assert response.status==200
print('native OTLP consumer accepted')`
	var out bytes.Buffer
	if err := runWithIO(ctx, []string{"app", "exec", "api", "python", "-c", script, payload, actor}, &out, &out); err != nil {
		t.Fatalf("actual native OTLP consumer failed in %s: %v", actor, err)
	}
	if !strings.Contains(out.String(), "native OTLP consumer accepted") {
		t.Fatal("native OTLP acceptance marker missing")
	}
}

func nativeOTLPMaterialSnapshot(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture material redirected")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = sha256.Sum256(data)
		return nil
	})
	if err != nil || len(result) == 0 {
		t.Fatal("material snapshot unavailable", err)
	}
	return result
}

func assertNativeOTLPMaterialSnapshot(t *testing.T, expected map[string][32]byte) {
	t.Helper()
	for path, digest := range expected {
		data, err := os.ReadFile(path)
		if err != nil || sha256.Sum256(data) != digest {
			t.Fatal("original protected material changed or disappeared")
		}
	}
}

// This receiver is fixture-owned independently of the application Target.
// Its native DNS alias and the host fixture DNS select the same TLS authority;
// no client transport, certificate verification or production resolver changes.
func startNativeOTLPReceiver(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider) nativeOTLPReceiver {
	t.Helper()
	root := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	host := "receiver.fixture.test"
	issuer := serviceissuer.New(t)
	server, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: host, DNSNames: []string{host}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{CommonName: "native-workload-client"})
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte, mode os.FileMode) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ca := write("ca.pem", server.IssuingCA, 0644)
	serverCert := write("server.pem", server.Certificate, 0644)
	serverKey := write("server-key.pem", server.PrivateKey, 0600)
	clientCert := write("client.pem", client.Certificate, 0644)
	clientKey := write("client-key.pem", client.PrivateKey, 0600)
	credential := write("credential.json", []byte(`{"token":"native-fixture-receiver-token"}`), 0600)
	code := write("receiver.py", []byte(nativeOTLPReceiverPython), 0644)
	project := "otlp-fixture-" + strings.ReplaceAll(application.MustNewApplicationID(), "-", "")[:12]
	receiver := nativeOTLPReceiver{host: host, project: project, network: project + "_receiver", compose: filepath.Join(root, "compose.yaml"), env: filepath.Join(root, "runtime.env")}
	receiver.registration = externalprovider.Registration{ID: "native-receiver", ProviderID: "fixture/independent-otlp", Provider: capability.ExternalOTLP, Endpoint: "https://" + net.JoinHostPort(host, strconv.Itoa(port)), CredentialRef: credential,
		Trust: externalprovider.Trust{Mode: externalprovider.TrustMTLS, CAReference: ca, ClientCertificate: clientCert, ClientKey: clientKey}}
	compose := fmt.Sprintf("services:\n  receiver:\n    image: docker.io/library/python:3.13-alpine\n    read_only: true\n    cap_drop: [ALL]\n    security_opt: [no-new-privileges:true]\n    tmpfs: [/tmp]\n    command: [python, -u, /receiver.py, %q]\n    ports: [%q]\n    volumes:\n      - %q\n      - %q\n      - %q\n      - %q\n      - %q\n    networks:\n      receiver:\n        aliases: [%q]\nnetworks:\n  receiver:\n    name: %q\n", strconv.Itoa(port), fmt.Sprintf("127.0.0.1:%d:%d", port, port), code+":/receiver.py:ro", ca+":/ca.pem:ro", serverCert+":/server.pem:ro", serverKey+":/server-key.pem:ro", credential+":/credential.json:ro", host, receiver.network)
	if err := os.WriteFile(receiver.compose, []byte(compose), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiver.env, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if err := runtime.DestroyProject(cleanup, project, receiver.compose, receiver.env); err != nil {
			t.Errorf("independent fixture receiver cleanup: %v", err)
		}
	})
	if err := runtime.ConfigProject(ctx, project, receiver.compose, receiver.env); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpProject(ctx, project, receiver.compose, receiver.env); err != nil {
		t.Fatal(err)
	}
	receiver.material = nativeOTLPMaterialSnapshot(t, root)
	return receiver
}
