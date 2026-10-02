package application

import (
	"context"
	"fmt"
	"strings"
)

type valkeyHAProbeRuntime interface {
	Run(context.Context, string, ...string) (string, error)
	RunSensitive(context.Context, string, []byte, ...string) (string, error)
}

func VerifyValkeyHACluster(ctx context.Context, runtime valkeyHAProbeRuntime, m Manifest, files RuntimeFiles) error {
	if runtime == nil {
		return fmt.Errorf("Valkey HA verification requires a runtime provider")
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range ValkeyInstanceNames(m) {
		if valkeyMemberCount(m, instance) <= 1 {
			continue
		}
		password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return err
		}
		masterCount := 0
		var observedMaster string
		for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
			service := valkeyMemberServiceName(instance, ordinal)
			script := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli -h 127.0.0.1 -p 6379 info replication"
			out, err := runtime.RunSensitive(ctx, service, []byte(password+"\n"), "sh", "-ec", script)
			if err != nil {
				return fmt.Errorf("inspect Valkey HA member %s: %w", service, err)
			}
			role := parseValkeyReplicationRole(out)
			switch role {
			case "master":
				masterCount++
				observedMaster = service
			case "slave", "replica":
			default:
				return fmt.Errorf("Valkey HA member %s reported unsupported replication role %q", service, role)
			}
		}
		if masterCount != 1 {
			return fmt.Errorf("Valkey HA instance %s has %d masters, require exactly one", instance, masterCount)
		}

		for ordinal := 0; ordinal < 3; ordinal++ {
			sentinel := valkeySentinelServiceName(instance, ordinal)
			out, err := runtime.Run(ctx, sentinel, "valkey-cli", "-p", "26379", "SENTINEL", "get-master-addr-by-name", valkeySentinelMasterName)
			if err != nil {
				return fmt.Errorf("inspect Valkey Sentinel %s: %w", sentinel, err)
			}
			lines := nonEmptyLines(out)
			if len(lines) < 2 {
				return fmt.Errorf("Valkey Sentinel %s returned incomplete master address", sentinel)
			}
			if lines[0] != observedMaster {
				return fmt.Errorf("Valkey Sentinel %s reports master %s, observed primary is %s", sentinel, lines[0], observedMaster)
			}
			if lines[1] != "6379" {
				return fmt.Errorf("Valkey Sentinel %s reports unexpected master port %s", sentinel, lines[1])
			}
		}
	}
	return nil
}

func parseValkeyReplicationRole(out string) string {
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "role:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "role:"))
		}
	}
	return ""
}

func nonEmptyLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
