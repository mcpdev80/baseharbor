package application

import (
	"fmt"
	"strconv"
)

// ReallocateRuntimePorts replaces only the generated host-port assignments for
// an already materialized application runtime. Credentials, databases, volumes
// and other persisted runtime values are left unchanged. This is used when the
// container runtime proves that a probed loopback port was claimed between
// allocation and the actual container bind.
func ReallocateRuntimePorts(m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}

	excluded := map[int]struct{}{}
	for _, instance := range PostgresInstanceNames(m) {
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return err
		}
		values[postgresRuntimeKey(instance, "HOST_PORT")] = strconv.Itoa(port)
		excluded[port] = struct{}{}
	}
	for _, instance := range RedisInstanceNames(m) {
		port, err := allocateLoopbackPort(excluded)
		if err != nil {
			return err
		}
		values[valkeyRuntimeKey(instance, "HOST_PORT")] = strconv.Itoa(port)
		excluded[port] = struct{}{}
	}

	if err := validateRuntimeValues(values, m); err != nil {
		return err
	}
	if err := writeRuntimeEnv(files.Env, m, values); err != nil {
		return err
	}
	if _, err := EnsureRuntimeContract(m, files); err != nil {
		return fmt.Errorf("refresh application contract after host-port reallocation: %w", err)
	}
	return nil
}
