package runtimebroker

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"go.yaml.in/yaml/v3"
)

func TestBrokerOwnedInventoryMatchesRendererAndExcludesExternalNetworks(t *testing.T) {
	for _, mode := range []string{"secrets", "operations", "metrics", "application-backend"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name string) string {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte("isolated fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				return path
			}
			m := application.New("example", "dev", false, false, true)
			if mode == "operations" {
				m = application.WithRuntimePermission(m, string(capability.ObjectStorageS3V1.ID), []string{"api"}, "runtime.get")
			}
			if mode == "metrics" {
				m = application.WithRuntimePermission(m, string(capability.MetricsV1.ID), []string{"api"}, "runtime.get")
			}
			if mode == "application-backend" {
				m.Services.SQL = true
				t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), string(capability.ScopeApplication))
			}
			files := application.RuntimeFiles{Namespace: "isolated", ResourceProject: "baseharbor-isolated-example-dev"}
			mtls := openbao.RuntimeMTLSFiles{CA: write("ca.pem"), BrokerCert: write("broker-cert.pem"), BrokerKey: write("broker-key.pem"), ClientCert: write("probe-client.pem"), ClientKey: write("probe-key.pem")}
			content, err := composeYAMLForRuntime(m, files, mtls, write("token"), write("credentials"), write("bao-ca"), write("permissions"), write("service-tokens"), "broker:test", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			var model struct {
				Services map[string]any `yaml:"services"`
				Volumes  map[string]struct {
					Name string `yaml:"name"`
				} `yaml:"volumes"`
				Networks map[string]struct {
					Name     string `yaml:"name"`
					External bool   `yaml:"external"`
				} `yaml:"networks"`
			}
			if err := yaml.Unmarshal([]byte(content), &model); err != nil {
				t.Fatal(err)
			}
			project := ProjectNameForRuntime(m, files)
			expected := []bhruntime.ProjectResource{}
			for service := range model.Services {
				expected = append(expected, bhruntime.ProjectResource{Kind: "container", Name: project + "-" + service + "-1"})
			}
			for key, volume := range model.Volumes {
				name := volume.Name
				if name == "" {
					name = project + "_" + key
				}
				expected = append(expected, bhruntime.ProjectResource{Kind: "volume", Name: name})
			}
			for _, network := range model.Networks {
				if !network.External {
					expected = append(expected, bhruntime.ProjectResource{Kind: "network", Name: network.Name})
				}
			}
			sort.Slice(expected, func(i, j int) bool { return expected[i].Kind+expected[i].Name < expected[j].Kind+expected[j].Name })
			actual, err := OwnedRuntimeResources(m, files)
			if err != nil || !reflect.DeepEqual(expected, actual) {
				t.Fatalf("canonical owner inventory differs from renderer: %+v %+v %v", expected, actual, err)
			}
		})
	}
}
