package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMaterializeManagementUIEndpointBindingIsReadableAndReplaceable(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQLManagementUI = true
	files := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	canonical := map[string]string{"sql": "https://pgadmin.baha.localhost:46811"}
	directIP := map[string]string{"sql": "https://192.0.2.8:52193"}
	if err := MaterializeManagementUIEndpointBinding(m, files, canonical, directIP); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(WorkloadServiceBindingProjectionDir(files), "management-ui")
	for _, item := range []struct {
		name string
		want map[string]string
	}{{"urls", canonical}, {"ip_urls", directIP}} {
		contents, err := os.ReadFile(filepath.Join(path, item.name))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]string
		if err := json.Unmarshal(contents, &got); err != nil {
			t.Fatal(err)
		}
		if got["sql"] != item.want["sql"] {
			t.Fatalf("%s binding = %#v, want %#v", item.name, got, item.want)
		}
		info, err := os.Stat(filepath.Join(path, item.name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o444 {
			t.Fatalf("%s mode = %v, want 0444", item.name, info.Mode().Perm())
		}
	}
	canonical["sql"] = "https://pgadmin.baha.localhost:49821"
	if err := MaterializeManagementUIEndpointBinding(m, files, canonical, directIP); err != nil {
		t.Fatalf("replace mounted endpoint binding: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(path, "urls"))
	if err != nil || !strings.Contains(string(contents), ":49821") {
		t.Fatalf("updated binding = %q, err=%v", contents, err)
	}
}

func TestManagementUIOnlyWorkloadReceivesStandardBindingMount(t *testing.T) {
	m := New("demo", "dev", false, false, false)
	m.Services.SQLManagementUI = true
	runtime := RuntimeFiles{Dir: filepath.Join(t.TempDir(), "runtime")}
	got, err := workloadOverrideYAMLForFiles(m, []string{"web"}, map[string]string{}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	wantMount := WorkloadServiceBindingProjectionDir(runtime) + ":" + workloadServiceBindingRoot + ":ro"
	if !strings.Contains(got, "SERVICE_BINDING_ROOT: "+strconv.Quote(workloadServiceBindingRoot)) || !strings.Contains(got, strconv.Quote(wantMount)) {
		t.Fatalf("management-UI-only workload lacks read-only binding mount %q:\n%s", wantMount, got)
	}
}
