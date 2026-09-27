package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type workloadReferenceRewriteKind string

const (
	workloadRewriteSQLURL      workloadReferenceRewriteKind = "sql-url"
	workloadRewriteSQLHostPort workloadReferenceRewriteKind = "sql-hostport"
	workloadRewriteCacheURL    workloadReferenceRewriteKind = "cache-url"
	workloadRewriteCacheHost   workloadReferenceRewriteKind = "cache-hostport"
)

type workloadReferenceRewrite struct {
	Service  string
	Variable string
	Kind     workloadReferenceRewriteKind
	Original string
}

func analyzeManagedServiceReferenceRewrites(m application.Manifest, rendered []byte, selectedServices []string) ([]workloadReferenceRewrite, error) {
	var config renderedComposeConfig
	if err := json.Unmarshal(rendered, &config); err != nil {
		return nil, fmt.Errorf("decode rendered application workload for managed-service references: %w", err)
	}
	selected := make(map[string]struct{}, len(selectedServices))
	for _, service := range selectedServices {
		selected[service] = struct{}{}
	}
	excluded := make(map[string]struct{})
	for service, definition := range config.Services {
		if _, ok := selected[service]; ok {
			continue
		}
		if isManagedReplacementService(m, service, definition.Image) {
			excluded[service] = struct{}{}
		}
	}
	if len(excluded) == 0 {
		return nil, nil
	}

	var rewrites []workloadReferenceRewrite
	for service, definition := range config.Services {
		if _, ok := selected[service]; !ok {
			continue
		}
		keys := make([]string, 0, len(definition.Environment))
		for key := range definition.Environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, ok := definition.Environment[key].(string)
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			kind, referenced, err := classifyManagedServiceReference(m, strings.TrimSpace(value), excluded)
			if err != nil {
				return nil, fmt.Errorf("workload service %s environment %s: %w", service, key, err)
			}
			if !referenced {
				continue
			}
			rewrites = append(rewrites, workloadReferenceRewrite{
				Service: service, Variable: key, Kind: kind, Original: value,
			})
		}
	}
	return rewrites, nil
}

func isManagedReplacementService(m application.Manifest, service, image string) bool {
	value := strings.ToLower(strings.TrimSpace(service + " " + image))
	postgres := strings.Contains(value, "postgres") || strings.Contains(value, "postgresql")
	cache := strings.Contains(value, "redis") || strings.Contains(value, "valkey")
	return (m.Services.SQL && postgres) || (m.Services.Cache && cache)
}

func classifyManagedServiceReference(m application.Manifest, value string, excluded map[string]struct{}) (workloadReferenceRewriteKind, bool, error) {
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Hostname() != "" {
		if _, ok := excluded[parsed.Hostname()]; !ok {
			return "", false, nil
		}
		switch strings.ToLower(parsed.Scheme) {
		case "postgres", "postgresql":
			if !m.Services.SQL {
				return "", true, fmt.Errorf("references excluded Compose service %q through PostgreSQL but managed SQL is not declared", parsed.Hostname())
			}
			if len(application.SQLInstanceNames(m)) != 1 {
				return "", true, fmt.Errorf("references excluded PostgreSQL service %q but %d managed SQL instances make automatic rewiring ambiguous", parsed.Hostname(), len(application.SQLInstanceNames(m)))
			}
			return workloadRewriteSQLURL, true, nil
		case "redis", "rediss":
			if !m.Services.Cache {
				return "", true, fmt.Errorf("references excluded Compose service %q through Redis but managed cache is not declared", parsed.Hostname())
			}
			if len(application.CacheInstanceNames(m)) != 1 {
				return "", true, fmt.Errorf("references excluded Redis service %q but %d managed cache instances make automatic rewiring ambiguous", parsed.Hostname(), len(application.CacheInstanceNames(m)))
			}
			return workloadRewriteCacheURL, true, nil
		default:
			return "", true, fmt.Errorf("references excluded Compose service %q through unsupported scheme %q; select the service or map the reference explicitly", parsed.Hostname(), parsed.Scheme)
		}
	}

	if host, port, err := net.SplitHostPort(value); err == nil {
		if _, ok := excluded[host]; !ok {
			return "", false, nil
		}
		switch port {
		case "5432":
			if !m.Services.SQL {
				return "", true, fmt.Errorf("references excluded Compose service %q on PostgreSQL port 5432 but managed SQL is not declared", host)
			}
			if len(application.SQLInstanceNames(m)) != 1 {
				return "", true, fmt.Errorf("references excluded PostgreSQL service %q but managed SQL instance selection is ambiguous", host)
			}
			return workloadRewriteSQLHostPort, true, nil
		case "6379":
			if !m.Services.Cache {
				return "", true, fmt.Errorf("references excluded Compose service %q on Redis port 6379 but managed cache is not declared", host)
			}
			if len(application.CacheInstanceNames(m)) != 1 {
				return "", true, fmt.Errorf("references excluded Redis service %q but managed cache instance selection is ambiguous", host)
			}
			return workloadRewriteCacheHost, true, nil
		default:
			return "", true, fmt.Errorf("references excluded Compose service %q on unsupported port %s; select the service or map the reference explicitly", host, port)
		}
	}
	if _, ok := excluded[value]; ok {
		return "", true, fmt.Errorf("references excluded Compose service %q without a protocol/port; automatic rewiring is unsafe", value)
	}
	return "", false, nil
}

func managedReferenceRewriteValue(kind workloadReferenceRewriteKind, managed map[string]string) (string, error) {
	key := ""
	switch kind {
	case workloadRewriteSQLURL, workloadRewriteSQLHostPort:
		key = "DATABASE_URL"
	case workloadRewriteCacheURL, workloadRewriteCacheHost:
		key = "REDIS_URL"
	default:
		return "", fmt.Errorf("unsupported managed reference rewrite kind %q", kind)
	}
	value := strings.TrimSpace(managed[key])
	if value == "" {
		return "", fmt.Errorf("managed workload environment is missing %s", key)
	}
	if kind == workloadRewriteSQLURL || kind == workloadRewriteCacheURL {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("managed %s is not a valid connection URL", key)
	}
	return parsed.Host, nil
}

func materializeManagedServiceReferenceRewrite(
	ctx context.Context,
	compose bhruntime.Compose,
	resolved resolvedApplication,
	workload application.WorkloadFiles,
	files application.RuntimeFiles,
	environment map[string]string,
) (string, bool, error) {
	rendered, err := compose.ConfigJSONProjectFilesEnv(ctx, workload.Project, workload.RepositoryRoot, environment, workload.Compose)
	if err != nil {
		return "", false, fmt.Errorf("render repository Compose for managed-service rewiring: %w", err)
	}
	rewrites, err := analyzeManagedServiceReferenceRewrites(resolved.Manifest, []byte(rendered), workload.Services)
	if err != nil {
		return "", false, err
	}
	path := filepath.Join(files.Dir, "workload.managed-service-rewrite.override.yaml")
	if len(rewrites) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		return "", false, nil
	}
	managed, err := application.ManagedWorkloadEnvironment(resolved.Manifest, files)
	if err != nil {
		return "", false, fmt.Errorf("resolve managed workload environment for rewiring: %w", err)
	}

	byService := map[string]map[string]string{}
	for _, rewrite := range rewrites {
		value, err := managedReferenceRewriteValue(rewrite.Kind, managed)
		if err != nil {
			return "", false, fmt.Errorf("rewrite %s/%s: %w", rewrite.Service, rewrite.Variable, err)
		}
		if byService[rewrite.Service] == nil {
			byService[rewrite.Service] = map[string]string{}
		}
		byService[rewrite.Service][rewrite.Variable] = value
	}
	services := make([]string, 0, len(byService))
	for service := range byService {
		services = append(services, service)
	}
	sort.Strings(services)
	var b strings.Builder
	b.WriteString("services:\n")
	for _, service := range services {
		fmt.Fprintf(&b, "  %s:\n    environment:\n", strconv.Quote(service))
		keys := make([]string, 0, len(byService[service]))
		for key := range byService[service] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "      %s: %s\n", strconv.Quote(key), strconv.Quote(byService[service][key]))
		}
	}
	if err := os.MkdirAll(files.Dir, 0o700); err != nil {
		return "", false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	return path, true, nil
}

