package runtime

import (
	"context"
	"fmt"
	"strings"
)

type LogCollectionMode string

const (
	LogCollectionSyslog   LogCollectionMode = "syslog"
	LogCollectionJournald LogCollectionMode = "journald"
)

func (Compose) LogCollectionMode() LogCollectionMode {
	return LogCollectionSyslog
}

// VerifyProjectServiceLogCollection keeps runtime-product verification behind
// the runtime provider boundary. Docker verifies the configured syslog driver
// and tag. Podman/Quadlet uses journald and has no equivalent Compose log-tag
// contract to assert here.
func (c Compose) VerifyProjectServiceLogCollection(ctx context.Context, project, service, expectedTag string) error {
	switch c.LogCollectionMode() {
	case LogCollectionJournald:
		return nil
	case LogCollectionSyslog:
		driver, tag, err := c.ContainerLogConfigProjectService(ctx, project, service)
		if err != nil {
			return err
		}
		if driver != "syslog" {
			return fmt.Errorf("runtime log driver for %s/%s: got %q, want syslog", project, service, driver)
		}
		if expectedTag = strings.TrimSpace(expectedTag); expectedTag != "" && tag != expectedTag {
			return fmt.Errorf("runtime syslog tag for %s/%s: got %q, want %q", project, service, tag, expectedTag)
		}
		return nil
	default:
		return fmt.Errorf("unsupported runtime log collection mode %q", c.LogCollectionMode())
	}
}
