package connectivityrelay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const DefaultImage = "ghcr.io/mcpdev80/baseharbor-runtime:edge"

type RuntimeSpec struct {
	ID            string
	SourceNetwork string
	SourceAlias   string
	TargetNetwork string
	TargetHost    string
	TargetPort    int
}

type Files struct {
	Dir     string
	Compose string
	Env     string
	Project string
}

func EnsureFiles(spec RuntimeSpec) (Files, error) {
	if strings.TrimSpace(spec.ID) == "" {
		return Files{}, errors.New("connectivity relay id is required")
	}
	for label, value := range map[string]string{
		"source network": spec.SourceNetwork,
		"source alias":   spec.SourceAlias,
		"target network": spec.TargetNetwork,
		"target host":    spec.TargetHost,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return Files{}, fmt.Errorf("%s is invalid", label)
		}
	}
	if spec.TargetPort < 1 || spec.TargetPort > 65535 {
		return Files{}, errors.New("connectivity relay target port is invalid")
	}
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Files{}, err
	}
	dir := filepath.Join(dataDir, "connectivity", spec.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Files{}, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Files{}, err
	}
	image := strings.TrimSpace(os.Getenv("BASEHARBOR_RUNTIME_IMAGE"))
	if image == "" {
		image = DefaultImage
	}
	if strings.ContainsAny(image, "\r\n\x00") {
		return Files{}, errors.New("BASEHARBOR_RUNTIME_IMAGE is invalid")
	}
	files := Files{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Project: "baseharbor-connectivity-" + spec.ID,
	}
	if err := os.WriteFile(files.Env, nil, 0o600); err != nil {
		return Files{}, err
	}
	if err := os.Chmod(files.Env, 0o600); err != nil {
		return Files{}, err
	}
	if err := os.WriteFile(files.Compose, []byte(composeYAML(spec, image)), 0o600); err != nil {
		return Files{}, err
	}
	if err := os.Chmod(files.Compose, 0o600); err != nil {
		return Files{}, err
	}
	return files, nil
}

func RemoveFiles(id string) error {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("connectivity relay id is required")
	}
	return os.RemoveAll(filepath.Join(dataDir, "connectivity", id))
}

func ExistingFiles(id string) (Files, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return Files{}, err
	}
	dir := filepath.Join(dataDir, "connectivity", strings.TrimSpace(id))
	files := Files{
		Dir:     dir,
		Compose: filepath.Join(dir, "compose.yaml"),
		Env:     filepath.Join(dir, "runtime.env"),
		Project: "baseharbor-connectivity-" + strings.TrimSpace(id),
	}
	for _, path := range []string{files.Compose, files.Env} {
		if _, err := os.Stat(path); err != nil {
			return Files{}, err
		}
	}
	return files, nil
}

func composeYAML(spec RuntimeSpec, image string) string {
	listen := "0.0.0.0:" + strconv.Itoa(spec.TargetPort)
	target := spec.TargetHost + ":" + strconv.Itoa(spec.TargetPort)
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  relay:\n")
	fmt.Fprintf(&b, "    image: %s\n", strconv.Quote(image))
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    command: [\"serve\"]\n")
	b.WriteString("    environment:\n")
	b.WriteString("      BASEHARBOR_CONNECTIVITY_RELAY_MODE: \"true\"\n")
	fmt.Fprintf(&b, "      BASEHARBOR_RELAY_LISTEN_ADDR: %s\n", strconv.Quote(listen))
	fmt.Fprintf(&b, "      BASEHARBOR_RELAY_TARGET_ADDR: %s\n", strconv.Quote(target))
	b.WriteString("      BASEHARBOR_RELAY_HEALTH_ADDR: \"127.0.0.1:8081\"\n")
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"curl\", \"--fail\", \"--silent\", \"--show-error\", \"http://127.0.0.1:8081/readyz\"]\n")
	b.WriteString("      interval: 2s\n")
	b.WriteString("      timeout: 2s\n")
	b.WriteString("      retries: 15\n")
	b.WriteString("      start_period: 1s\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - \"/tmp:rw,noexec,nosuid,nodev,size=8m\"\n")
	b.WriteString("    cap_drop:\n")
	b.WriteString("      - ALL\n")
	b.WriteString("    security_opt:\n")
	b.WriteString("      - \"no-new-privileges:true\"\n")
	b.WriteString("    networks:\n")
	b.WriteString("      source:\n")
	b.WriteString("        aliases:\n")
	fmt.Fprintf(&b, "          - %s\n", strconv.Quote(spec.SourceAlias))
	b.WriteString("      target: {}\n")
	b.WriteString("networks:\n")
	b.WriteString("  source:\n")
	b.WriteString("    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(spec.SourceNetwork))
	b.WriteString("  target:\n")
	b.WriteString("    external: true\n")
	fmt.Fprintf(&b, "    name: %s\n", strconv.Quote(spec.TargetNetwork))
	return b.String()
}
