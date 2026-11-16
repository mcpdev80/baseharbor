package podman

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"
)

// Observe the same native CLI selection and environment used by this provider,
// rather than proving an alternate --remote=false engine. Podman documents
// host.serviceIsRemote and host.security.rootless in its info JSON contract:
// https://docs.podman.io/en/latest/markdown/podman-info.1.html
func (p Provider) VerifyLocalRuntimeAuthority(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	path, err := exec.LookPath("podman")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "info", "--format", "json")
	cmd.Env = runtimeCommandEnv(path)
	data, err := cmd.Output()
	if err != nil {
		return errors.New("selected Podman engine authority could not be observed")
	}
	return verifyLocalAuthorityInfo(data)
}

func verifyLocalAuthorityInfo(data []byte) error {
	var info struct {
		Host struct {
			OS       string `json:"os"`
			Remote   *bool  `json:"serviceIsRemote"`
			Security struct {
				Rootless *bool `json:"rootless"`
			} `json:"security"`
		} `json:"host"`
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &info) != nil || info.Host.OS != "linux" || info.Host.Remote == nil || *info.Host.Remote || info.Host.Security.Rootless == nil || !*info.Host.Security.Rootless {
		return errors.New("selected Podman engine is not a verified native rootless Linux authority")
	}
	return nil
}
