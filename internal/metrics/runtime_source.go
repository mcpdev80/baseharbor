package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

const RuntimeCapabilityV1 = "metrics/v1"

type RuntimeSourceExecutor struct {
	environment string
	targetsDir  string
}

func NewRuntimeSourceExecutor(environment, targetsDir string) (*RuntimeSourceExecutor, error) {
	environment = strings.TrimSpace(environment)
	targetsDir = strings.TrimSpace(targetsDir)
	if environment == "" || targetsDir == "" {
		return nil, errors.New("runtime metrics executor requires environment and target directory")
	}
	if err := os.MkdirAll(targetsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime metrics target directory: %w", err)
	}
	if err := os.Chmod(targetsDir, 0o700); err != nil {
		return nil, fmt.Errorf("protect runtime metrics target directory: %w", err)
	}
	return &RuntimeSourceExecutor{environment: environment, targetsDir: targetsDir}, nil
}

func (e *RuntimeSourceExecutor) Execute(_ context.Context, request runtimeoperation.Request) (runtimeoperation.Result, error) {
	if strings.TrimSpace(request.Capability) != RuntimeCapabilityV1 {
		return runtimeoperation.Result{}, errors.New("unsupported runtime metrics capability")
	}
	if strings.TrimSpace(request.Application) == "" || strings.TrimSpace(request.CallerService) == "" || strings.TrimSpace(request.ResourceName) == "" {
		return runtimeoperation.Result{}, errors.New("runtime metrics source identity is incomplete")
	}
	switch request.Operation {
	case "runtime.create":
		source, err := parseRuntimeSource(request)
		if err != nil {
			return runtimeoperation.Result{}, err
		}
		if err := e.writeTarget(request, source); err != nil {
			return runtimeoperation.Result{}, err
		}
		return runtimeoperation.Result{ResourceID: runtimeMetricsResourceID(request), Binding: source.binding(request)}, nil
	case "runtime.get":
		source, err := e.readTarget(request)
		if err != nil {
			return runtimeoperation.Result{}, err
		}
		return runtimeoperation.Result{ResourceID: runtimeMetricsResourceID(request), Binding: source.binding(request)}, nil
	case "runtime.delete":
		path := e.targetPath(request)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return runtimeoperation.Result{}, fmt.Errorf("delete runtime metrics source: %w", err)
		}
		return runtimeoperation.Result{ResourceID: runtimeMetricsResourceID(request)}, nil
	default:
		return runtimeoperation.Result{}, errors.New("unsupported runtime metrics operation")
	}
}

type runtimeSource struct {
	Port int
	Path string
}

func parseRuntimeSource(request runtimeoperation.Request) (runtimeSource, error) {
	if request.Parameters == nil {
		return runtimeSource{}, errors.New("runtime metrics source parameters are required")
	}
	for key := range request.Parameters {
		if key != "port" && key != "path" {
			return runtimeSource{}, fmt.Errorf("runtime metrics parameter %q is not allowed", key)
		}
	}
	rawPort, ok := request.Parameters["port"]
	if !ok {
		return runtimeSource{}, errors.New("runtime metrics port is required")
	}
	port, err := numericPort(rawPort)
	if err != nil {
		return runtimeSource{}, err
	}
	path, _ := request.Parameters["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/metrics"
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\r\n\x00") {
		return runtimeSource{}, errors.New("runtime metrics path must be an absolute HTTP path without query or fragment")
	}
	return runtimeSource{Port: port, Path: path}, nil
}

func numericPort(value any) (int, error) {
	var port int
	switch typed := value.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return 0, errors.New("runtime metrics port must be an integer")
		}
		port = int(typed)
	case int:
		port = typed
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err != nil {
			return 0, errors.New("runtime metrics port must be an integer")
		}
		port = parsed
	default:
		return 0, errors.New("runtime metrics port must be an integer")
	}
	if port < 1 || port > 65535 {
		return 0, errors.New("runtime metrics port must be between 1 and 65535")
	}
	return port, nil
}

func (e *RuntimeSourceExecutor) writeTarget(request runtimeoperation.Request, source runtimeSource) error {
	group := targetGroup{
		Targets: []string{net.JoinHostPort(application.MetricsTargetAliasFor(request.Application, e.environment, request.CallerService), strconv.Itoa(source.Port))},
		Labels: map[string]string{
			"job":                     "baseharbor-runtime-metrics",
			"baseharbor_application":  request.Application,
			"baseharbor_environment":  e.environment,
			"baseharbor_service":      request.CallerService,
			"baseharbor_source":       request.ResourceName,
			"baseharbor_source_class": string(application.MetricsSourceApplication),
			"baseharbor_metrics_path": source.Path,
		},
	}
	data, err := json.MarshalIndent([]targetGroup{group}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := e.targetPath(request)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write runtime metrics target: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit runtime metrics target: %w", err)
	}
	return nil
}

func (e *RuntimeSourceExecutor) readTarget(request runtimeoperation.Request) (runtimeSource, error) {
	data, err := os.ReadFile(e.targetPath(request))
	if errors.Is(err, os.ErrNotExist) {
		return runtimeSource{}, runtimeoperation.ErrNotFound
	}
	if err != nil {
		return runtimeSource{}, err
	}
	var groups []targetGroup
	if err := json.Unmarshal(data, &groups); err != nil || len(groups) != 1 || len(groups[0].Targets) != 1 {
		return runtimeSource{}, errors.New("runtime metrics target state is invalid")
	}
	target := groups[0].Targets[0]
	_, rawPort, err := net.SplitHostPort(target)
	if err != nil {
		return runtimeSource{}, errors.New("runtime metrics target state has invalid endpoint")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return runtimeSource{}, errors.New("runtime metrics target state has invalid port")
	}
	path := strings.TrimSpace(groups[0].Labels["baseharbor_metrics_path"])
	if path == "" {
		return runtimeSource{}, errors.New("runtime metrics target state has invalid path")
	}
	if groups[0].Labels["baseharbor_application"] != request.Application ||
		groups[0].Labels["baseharbor_environment"] != e.environment ||
		groups[0].Labels["baseharbor_service"] != request.CallerService ||
		groups[0].Labels["baseharbor_source"] != request.ResourceName {
		return runtimeSource{}, errors.New("runtime metrics target ownership mismatch")
	}
	return runtimeSource{Port: port, Path: path}, nil
}

func (e *RuntimeSourceExecutor) targetPath(request runtimeoperation.Request) string {
	return filepath.Join(e.targetsDir, runtimeTargetFileName(request))
}

func runtimeTargetFileName(request runtimeoperation.Request) string {
	sum := sha256.Sum256([]byte(request.Application + "\x00" + request.CallerService + "\x00" + request.ResourceName))
	return "runtime-" + hex.EncodeToString(sum[:12]) + ".json"
}

func runtimeMetricsResourceID(request runtimeoperation.Request) string {
	sum := sha256.Sum256([]byte(request.Application + "\x00" + request.CallerService + "\x00" + request.ResourceName))
	return "res-metrics-" + hex.EncodeToString(sum[:12])
}

func (s runtimeSource) binding(request runtimeoperation.Request) map[string]any {
	return map[string]any{
		"format":       "openmetrics",
		"service":      request.CallerService,
		"port":         s.Port,
		"path":         s.Path,
		"source_class": string(application.MetricsSourceApplication),
	}
}
