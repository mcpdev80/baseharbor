package identityprovider

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type databaseReloadRuntime struct {
	recordingKeycloakRuntime
	services            []string
	users               []string
	commands            [][]string
	callBudgets         []time.Duration
	failOn              string
	failure             error
	cancel              context.CancelFunc
	remainingFailures   int
	failCommandContains string
}

func (r *databaseReloadRuntime) ExecProjectAs(ctx context.Context, _, _, _, service, user string, args ...string) (string, error) {
	r.services = append(r.services, service)
	r.users = append(r.users, user)
	r.commands = append(r.commands, append([]string(nil), args...))
	if deadline, ok := ctx.Deadline(); ok {
		r.callBudgets = append(r.callBudgets, time.Until(deadline))
	} else {
		r.callBudgets = append(r.callBudgets, 0)
	}
	commandMatches := r.failCommandContains == "" || strings.Contains(strings.Join(args, " "), r.failCommandContains)
	if service == r.failOn && commandMatches && r.remainingFailures != 0 {
		r.remainingFailures--
		if r.cancel != nil {
			r.cancel()
		}
		return "", r.failure
	}
	return "server signaled", nil
}

func TestKeycloakDatabaseReloadFailsClosed(t *testing.T) {
	files := KeycloakFiles{Project: "identity", Compose: "compose.yaml", Env: filepath.Join(t.TempDir(), "runtime.env")}
	if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "ha"}); err != nil {
		t.Fatal(err)
	}
	if err := reloadKeycloakDatabaseCertificates(context.Background(), &recordingKeycloakRuntime{}, files); err == nil {
		t.Fatal("database certificate rotation accepted a runtime without reload support")
	}
	failure := errors.New("database reload rejected")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &databaseReloadRuntime{failOn: "keycloak-db-member-2", failure: failure, cancel: cancel, remainingFailures: -1}
	if err := reloadKeycloakDatabaseCertificates(ctx, runtime, files); !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
		t.Fatalf("database reload failure = %v, want original failure", err)
	}
	if !reflect.DeepEqual(runtime.services, []string{"keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-2"}) {
		t.Fatalf("reload continued after a member rejected it: %v", runtime.services)
	}
}

func TestKeycloakDatabaseReloadWaitsForRecoveringMembers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runtime := &databaseReloadRuntime{failOn: "keycloak-db-member-1", failure: errors.New("database system is starting up"), remainingFailures: 1}
	env := filepath.Join(t.TempDir(), "runtime.env")
	if err := writeProtectedEnv(env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "ha"}); err != nil {
		t.Fatal(err)
	}
	if err := reloadKeycloakDatabaseCertificates(ctx, runtime, KeycloakFiles{Env: env}); err != nil {
		t.Fatal(err)
	}
	want := []string{"keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-2", "keycloak-db-member-2", "keycloak-db-member-3", "keycloak-db-member-3", "keycloak-db-member-3"}
	if !reflect.DeepEqual(runtime.services, want) {
		t.Fatalf("database reload did not wait for every member: got %v want %v", runtime.services, want)
	}
	paths := keycloakDatabaseTLSKeyPaths{
		certificate: "/run/baseharbor/db-tls-source/server.pem",
		sourceKey:   "/run/baseharbor/db-tls-source/server-key.pem",
		runtimeKey:  "/run/baseharbor/etcd-runtime/server-key.pem",
	}
	install := []string{"sh", "-ec", keycloakDatabaseTLSKeyInstallCommand(paths)}
	reload := []string{"sh", "-ec", keycloakDatabaseTLSReloadSignalCommand()}
	verify := []string{"sh", "-ec", keycloakDatabaseTLSServedCertificateCommand(paths)}
	wantCommands := [][]string{install, install, reload, verify, install, reload, verify, install, reload, verify}
	if !reflect.DeepEqual(runtime.commands, wantCommands) {
		t.Fatalf("each HA member must reload then verify its own served certificate (the first reload may retry): got %v want %v", runtime.commands, wantCommands)
	}
	if !reflect.DeepEqual(runtime.users, []string{"postgres", "postgres", "postgres", "postgres", "postgres", "postgres", "postgres", "postgres", "postgres", "postgres"}) {
		t.Fatalf("database reload users = %v, want postgres for every member attempt", runtime.users)
	}
}

func TestKeycloakDatabaseServedCertificateRetryDoesNotRepeatKeyInstall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	env := filepath.Join(t.TempDir(), "runtime.env")
	if err := writeProtectedEnv(env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "ha"}); err != nil {
		t.Fatal(err)
	}
	runtime := &databaseReloadRuntime{
		failOn:              "keycloak-db-member-1",
		failure:             errors.New("served certificate is not current yet"),
		remainingFailures:   1,
		failCommandContains: "openssl s_client",
	}
	if err := reloadKeycloakDatabaseCertificates(ctx, runtime, KeycloakFiles{Env: env}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.commands) < 4 || !strings.Contains(strings.Join(runtime.commands[0], " "), "mv -f") || !strings.Contains(strings.Join(runtime.commands[1], " "), "kill -HUP") || !strings.Contains(strings.Join(runtime.commands[2], " "), "openssl s_client") || !strings.Contains(strings.Join(runtime.commands[3], " "), "openssl s_client") {
		t.Fatalf("expected key install, one signal, and retried served-certificate probe; commands=%v", runtime.commands)
	}
	for i, budget := range runtime.callBudgets {
		if budget <= 0 || budget > 10*time.Second {
			t.Fatalf("runtime call %d must have a positive per-attempt deadline <= 10s, got %s", i, budget)
		}
	}
}

func TestKeycloakDatabaseServedCertificateFailureNamesPhaseAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := filepath.Join(t.TempDir(), "runtime.env")
	if err := writeProtectedEnv(env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "ha"}); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("probe unavailable")
	runtime := &databaseReloadRuntime{
		failOn:              "keycloak-db-member-1",
		failure:             failure,
		remainingFailures:   -1,
		cancel:              cancel,
		failCommandContains: "openssl s_client",
	}
	err := reloadKeycloakDatabaseCertificates(ctx, runtime, KeycloakFiles{Env: env})
	cancel()
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "verify served certificate") {
		t.Fatalf("served-certificate error must identify its phase and preserve the cause: %v", err)
	}
	if !reflect.DeepEqual(runtime.services, []string{"keycloak-db-member-1", "keycloak-db-member-1", "keycloak-db-member-1"}) {
		t.Fatalf("verification failure must stop before another member and must not repeat reload: %v", runtime.services)
	}
}

func TestKeycloakSingleDatabaseReload(t *testing.T) {
	env := filepath.Join(t.TempDir(), "runtime.env")
	if err := writeProtectedEnv(env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "single"}); err != nil {
		t.Fatal(err)
	}
	runtime := &databaseReloadRuntime{}
	if err := reloadKeycloakDatabaseCertificates(context.Background(), runtime, KeycloakFiles{Env: env}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runtime.services, []string{"keycloak-db", "keycloak-db", "keycloak-db"}) {
		t.Fatalf("single topology attempted HA reload: %v", runtime.services)
	}
	if got := runtime.commands[0][2]; !strings.Contains(got, `source_cert="/run/baseharbor/db-tls/server.pem"`) || !strings.Contains(got, `source_key="/run/baseharbor/db-tls/server-key.pem"`) || !strings.Contains(got, `runtime_key="/tmp/server-key.pem"`) {
		t.Fatalf("single topology did not refresh the copied PostgreSQL key from its mounted source: %s", got)
	}
	if !strings.Contains(runtime.commands[1][2], `kill -HUP "$pid"`) {
		t.Fatalf("single topology did not signal PostgreSQL after key installation: %s", runtime.commands[1][2])
	}
	if !strings.Contains(runtime.commands[2][2], `source_cert="/run/baseharbor/db-tls/server.pem"`) {
		t.Fatalf("single topology did not verify the served certificate against its projected certificate: %s", runtime.commands[2][2])
	}
}

func TestKeycloakDatabaseTLSReloadCommandsValidateInstallThenSignal(t *testing.T) {
	command := keycloakDatabaseTLSKeyInstallCommand(keycloakDatabaseTLSKeyPaths{
		certificate: "/run/baseharbor/db-tls-source/server.pem",
		sourceKey:   "/run/baseharbor/db-tls-source/server-key.pem",
		runtimeKey:  "/run/baseharbor/etcd-runtime/server-key.pem",
	})
	ordered := []string{
		`[ ! -L "$runtime_key" ]`,
		`cp "$source_key" "$tmp_key"`,
		`chmod 0600 "$tmp_key"`,
		`openssl x509 -in "$source_cert" -pubkey -noout > "$cert_pub"`,
		`openssl pkey -in "$tmp_key" -pubout > "$key_pub"`,
		`cmp -s "$cert_pub" "$key_pub" || { echo 'postgres server certificate and key do not match' >&2; exit 1; }`,
		`mv -f "$tmp_key" "$runtime_key"`,
	}
	last := -1
	for _, fragment := range ordered {
		at := strings.Index(command, fragment)
		if at <= last {
			t.Fatalf("key install command must validate and atomically install the matching key; fragment %q at %d after %d in:\n%s", fragment, at, last, command)
		}
		last = at
	}
	if strings.Contains(command, `kill -HUP`) {
		t.Fatalf("key install and PostgreSQL signal must be separate bounded runtime calls:\n%s", command)
	}
	signal := keycloakDatabaseTLSReloadSignalCommand()
	if !strings.Contains(signal, `postgres postmaster pid is not ready`) || !strings.Contains(signal, `postgres process is not ready`) || !strings.Contains(signal, `kill -HUP "$pid"`) {
		t.Fatalf("signal command must validate and signal only the live postgres process:\n%s", signal)
	}
	verify := keycloakDatabaseTLSServedCertificateCommand(keycloakDatabaseTLSKeyPaths{certificate: "/run/baseharbor/db-tls-source/server.pem"})
	for _, fragment := range []string{
		`timeout 5 openssl s_client -starttls postgres -connect 127.0.0.1:5432 -showcerts`,
		`sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' "$probe_output" > "$served_cert"`,
		`expected_fingerprint=$(openssl x509 -in "$source_cert" -noout -fingerprint -sha256)`,
		`served_fingerprint=$(openssl x509 -in "$served_cert" -noout -fingerprint -sha256)`,
		`[ "$served_fingerprint" = "$expected_fingerprint" ] || { echo "postgres still serves a stale certificate`,
	} {
		if !strings.Contains(verify, fragment) {
			t.Fatalf("served-certificate command lacks verification step %q:\n%s", fragment, verify)
		}
	}
}

type keycloakTrustReloadRecorder struct {
	services []string
	ready    []string
}

func (r *keycloakTrustReloadRecorder) UpProjectFilesSelectedForceRecreateNoBuild(_ context.Context, _, _ string, _ map[string]string, services []string, _ ...string) error {
	if len(services) != 1 {
		return errors.New("trust reload must recreate one Keycloak member at a time")
	}
	if len(r.services) > 0 && len(r.ready) < len(r.services) {
		return errors.New("next Keycloak member recreated before the previous member became ready")
	}
	r.services = append(r.services, services[0])
	return nil
}

func (r *keycloakTrustReloadRecorder) ServiceStatesProjectFilesEnv(_ context.Context, _, _ string, _ map[string]string, _ ...string) ([]bhruntime.ServiceState, error) {
	if len(r.services) == 0 {
		return nil, errors.New("service state read before Keycloak member recreation")
	}
	service := r.services[len(r.services)-1]
	if len(r.ready) < len(r.services) {
		r.ready = append(r.ready, service)
	}
	return []bhruntime.ServiceState{{Service: service, State: "running", Health: "healthy"}}, nil
}

func TestKeycloakDatabaseTrustReloadRollsMembersAndWaitsForReadiness(t *testing.T) {
	for _, topology := range []struct {
		name string
		want []string
	}{
		{name: "single", want: []string{"keycloak-1"}},
		{name: "ha", want: []string{"keycloak-1", "keycloak-2", "keycloak-3"}},
	} {
		t.Run(topology.name, func(t *testing.T) {
			files := KeycloakFiles{Project: "identity", Dir: t.TempDir(), Compose: "compose.yaml", Env: filepath.Join(t.TempDir(), "runtime.env")}
			if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": topology.name}); err != nil {
				t.Fatal(err)
			}
			runtime := &keycloakTrustReloadRecorder{}
			if err := reloadKeycloakDatabaseTrust(context.Background(), runtime, files); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(runtime.services, topology.want) || !reflect.DeepEqual(runtime.ready, topology.want) {
				t.Fatalf("database trust reload order: restarted=%v ready=%v want=%v", runtime.services, runtime.ready, topology.want)
			}
		})
	}
}

func TestKeycloakDatabaseTrustReloadRejectsUnknownTopology(t *testing.T) {
	files := KeycloakFiles{Project: "identity", Dir: t.TempDir(), Compose: "compose.yaml", Env: filepath.Join(t.TempDir(), "runtime.env")}
	if err := writeProtectedEnv(files.Env, map[string]string{"BASEHARBOR_KEYCLOAK_TOPOLOGY": "unknown"}); err != nil {
		t.Fatal(err)
	}
	runtime := &keycloakTrustReloadRecorder{}
	if err := reloadKeycloakDatabaseTrust(context.Background(), runtime, files); err == nil || len(runtime.services) != 0 {
		t.Fatalf("unknown Keycloak topology was not rejected before mutation: services=%v err=%v", runtime.services, err)
	}
}
