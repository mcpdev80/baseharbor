package application

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestMongoDBManagementUIRuntimeAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_MONGODB_UI_ACCEPTANCE") != "1" {
		t.Skip("MongoDB management UI acceptance requires BASEHARBOR_MONGODB_UI_ACCEPTANCE=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	runtime, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(t.TempDir(), "apps"), Namespace: "mongodb-ui-acceptance"}
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "mongodb-ui-ci",
		Environment:   "dev",
		Services: Services{
			DocumentDatabase:             true,
			DocumentDatabaseManagementUI: true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	files, err := EnsureRuntime(ctx, serviceissuer.New(t), store, m)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise a valid credential that must never be parsed as a mongosh option.
	initialEnv, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	key := mongodbRuntimeKey(defaultServiceInstance, "ADMIN_PASSWORD")
	initialEnv[key] = "-" + initialEnv[key]
	if err := writeRuntimeEnv(files.Env, m, initialEnv); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() && os.Getenv("BASEHARBOR_MONGODB_UI_KEEP_ON_FAILURE") == "1" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		if err := runtime.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env); err != nil {
			t.Errorf("destroy MongoDB UI runtime: %v", err)
		}
	}()

	deadline := time.Now().Add(120 * time.Second)
	for {
		err = VerifyApplicationManagementUIs(ctx, m, files)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MongoDB management UI never became ready: %v", err)
		}
		time.Sleep(time.Second)
	}

	surfaces, err := ApplicationManagementUISurfaces(m, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(surfaces) != 1 || surfaces[0].Service != "mongodb" || surfaces[0].Authentication != "http-basic" {
		t.Fatalf("unexpected MongoDB management surfaces: %#v", surfaces)
	}

	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(values[mongodbUIHostPortKey(defaultServiceInstance)])
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve(m.Environment, "mongodb-management", serviceaccess.AuthenticationNative)
	if err != nil {
		t.Fatal(err)
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "providers", "management-ui", "mongodb", defaultServiceInstance, "pki"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := serviceaccess.LoopbackHTTPSURL(port)
	if err != nil {
		t.Fatal(err)
	}

	noAuthClient := *client
	noAuthClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noAuthClient.Get(endpoint + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("MongoDB management UI without auth returned %d, want 401", resp.StatusCode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth(values[MongoDBUIUserEnv], values[MongoDBUIPasswordEnv])
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		t.Fatalf("MongoDB management UI authenticated request returned %d", resp.StatusCode)
	}

	running, err := runtime.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(running)
	want := []string{"mongodb", "mongodb-ui", "mongodb-ui-access"}
	if len(running) != len(want) {
		t.Fatalf("unexpected MongoDB UI runtime services: got %#v want %#v", running, want)
	}
	for i := range want {
		if running[i] != want[i] {
			t.Fatalf("unexpected MongoDB UI runtime services: got %#v want %#v", running, want)
		}
	}
}
