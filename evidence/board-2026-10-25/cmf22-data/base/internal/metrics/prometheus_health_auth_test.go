package metrics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	"go.yaml.in/yaml/v3"
	"golang.org/x/crypto/bcrypt"
)

func TestPrometheusManagementUIAuthenticatesMemberHealthChecks(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := application.Manifest{Name: "health-auth", Environment: "dev"}
	m.Services.ObservabilityManagementUI = true
	files, err := EnsureProviderFiles(context.Background(), serviceissuer.New(t), m)
	if err != nil {
		t.Fatal(err)
	}
	values, err := readPrometheusEnvironment(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(values["BASEHARBOR_PROMETHEUS_UI_USER"]+":"+values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"]))
	if values["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] != authorization {
		t.Fatal("health authorization does not use the current native UI credentials")
	}
	info, err := os.Stat(files.Env)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("credential environment must remain owner-only")
	}
	compose, err := os.ReadFile(files.Compose)
	if err != nil {
		t.Fatal(err)
	}
	var model struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(compose, &model); err != nil {
		t.Fatal(err)
	}
	if model.Services["prometheus-access"].Environment["BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION"] != "${BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION}" {
		t.Fatal("access gateway does not receive the health credential reference")
	}
	config, err := os.ReadFile(filepath.Join(files.Dir, "service-access", "config", "Caddyfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "Authorization \"{$BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION}\"") || !strings.Contains(string(config), "health_uri /-/ready") {
		t.Fatal("member readiness checks are not authenticated")
	}
	for _, public := range []string{string(config), string(compose)} {
		if strings.Contains(public, authorization) || strings.Contains(public, values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"]) {
			t.Fatal("generated public runtime configuration embeds health credentials")
		}
	}
	web, err := os.ReadFile(files.WebConfig)
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Users map[string]string `yaml:"basic_auth_users"`
	}
	if err := yaml.Unmarshal(web, &native); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(native.Users[values["BASEHARBOR_PROMETHEUS_UI_USER"]]), []byte(values["BASEHARBOR_PROMETHEUS_UI_PASSWORD"])); err != nil {
		t.Fatal("native management authentication was not retained")
	}
	if caddy := os.Getenv("BASEHARBOR_CADDY_TEST_BINARY"); caddy != "" {
		cmd := exec.Command(caddy, "adapt", "--config", filepath.Join(files.Dir, "service-access", "config", "Caddyfile"), "--adapter", "caddyfile")
		cmd.Env = append(os.Environ(), "BASEHARBOR_PROMETHEUS_HEALTH_AUTHORIZATION="+authorization)
		output, err := cmd.Output()
		if err != nil {
			t.Fatal("native Caddy rejected the generated configuration")
		}
		var adapted any
		if err := json.Unmarshal(output, &adapted); err != nil {
			t.Fatal(err)
		}
		found := false
		var inspect func(any)
		inspect = func(value any) {
			switch value := value.(type) {
			case map[string]any:
				if headers, ok := value["headers"].(map[string]any); ok {
					if items, ok := headers["Authorization"].([]any); ok && len(items) == 1 && items[0] == authorization {
						found = true
					}
				}
				for _, child := range value {
					inspect(child)
				}
			case []any:
				for _, child := range value {
					inspect(child)
				}
			}
		}
		inspect(adapted)
		if !found {
			t.Fatal("native Caddy did not preserve the complete health authorization header")
		}
	}
}
