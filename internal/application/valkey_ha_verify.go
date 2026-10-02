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
			observedMasterAddr, err := valkeyMemberAddressFromSentinel(ctx, runtime, sentinel, observedMaster, password)
			if err != nil {
				return fmt.Errorf("resolve observed Valkey primary %s from %s: %w", observedMaster, sentinel, err)
			}
			if lines[0] != observedMasterAddr {
				return fmt.Errorf("Valkey Sentinel %s reports master %s, observed primary %s resolves to %s", sentinel, lines[0], observedMaster, observedMasterAddr)
			}
			if lines[1] != "6379" {
				return fmt.Errorf("Valkey Sentinel %s reports unexpected master port %s", sentinel, lines[1])
			}
			quorum, err := runtime.Run(ctx, sentinel, "valkey-cli", "-p", "26379", "SENTINEL", "CKQUORUM", valkeySentinelMasterName)
			if err != nil {
				return fmt.Errorf("verify Valkey Sentinel quorum on %s: %w", sentinel, err)
			}
			if !strings.Contains(strings.ToUpper(quorum), "OK") {
				return fmt.Errorf("Valkey Sentinel %s cannot satisfy quorum/majority: %s", sentinel, strings.TrimSpace(quorum))
			}
		}
	}
	return nil
}

func ValkeyHAMaster(ctx context.Context, runtime valkeyHAProbeRuntime, m Manifest, files RuntimeFiles, instance string) (string, error) {
	if runtime == nil {
		return "", fmt.Errorf("Valkey HA master lookup requires a runtime provider")
	}
	if valkeyMemberCount(m, instance) <= 1 {
		return valkeyMemberServiceName(instance, 0), nil
	}
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}

	votes := map[string]int{}
	var observations []string
	for sentinelOrdinal := 0; sentinelOrdinal < 3; sentinelOrdinal++ {
		sentinel := valkeySentinelServiceName(instance, sentinelOrdinal)
		out, err := runtime.Run(ctx, sentinel, "valkey-cli", "-p", "26379", "SENTINEL", "get-master-addr-by-name", valkeySentinelMasterName)
		if err != nil {
			observations = append(observations, fmt.Sprintf("%s:error", sentinel))
			continue
		}
		lines := nonEmptyLines(out)
		if len(lines) < 2 || lines[1] != "6379" {
			observations = append(observations, fmt.Sprintf("%s:incomplete", sentinel))
			continue
		}
		reportedAddress := lines[0]
		mappedMember := ""
		for ordinal := 0; ordinal < valkeyMemberCount(m, instance); ordinal++ {
			member := valkeyMemberServiceName(instance, ordinal)
			address, addrErr := valkeyMemberAddressFromSentinel(ctx, runtime, sentinel, member, password)
			if addrErr != nil {
				continue
			}
			if reportedAddress == address {
				mappedMember = member
				break
			}
		}
		if mappedMember == "" {
			observations = append(observations, fmt.Sprintf("%s:%s(unmapped)", sentinel, reportedAddress))
			continue
		}
		votes[mappedMember]++
		observations = append(observations, fmt.Sprintf("%s:%s", sentinel, mappedMember))
		if votes[mappedMember] >= 2 {
			return mappedMember, nil
		}
	}
	return "", fmt.Errorf("Valkey Sentinel quorum has no mapped master majority: %s", strings.Join(observations, ", "))
}

func valkeyMemberAddressFromSentinel(ctx context.Context, runtime valkeyHAProbeRuntime, sentinel, member, password string) (string, error) {
	script := "IFS= read -r password; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli -h \"$1\" -p 6379 --raw CLIENT INFO"
	out, err := runtime.RunSensitive(ctx, sentinel, []byte(password+"\n"), "sh", "-ec", script, "sh", member)
	if err != nil {
		return "", err
	}
	for _, field := range strings.Fields(strings.ReplaceAll(out, "\r", "")) {
		if !strings.HasPrefix(field, "laddr=") {
			continue
		}
		hostPort := strings.TrimPrefix(field, "laddr=")
		idx := strings.LastIndex(hostPort, ":")
		if idx <= 0 {
			continue
		}
		return hostPort[:idx], nil
	}
	return "", fmt.Errorf("Valkey member %s CLIENT INFO has no laddr", member)
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
