package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func workstationCleanupCommand() *cli.Command {
	return &cli.Command{Name: "cleanup", Summary: "Remove this Target's owned workstation hosts and CA trust", Usage: "baha dev cleanup --yes", Long: "Local only; keeps Core, apps, data, device enrollment and unrelated workstation settings. Modified hosts entries/certificates are preserved. CAs still referenced by another connection are retained.", Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
		if len(args) != 1 || args[0] != "--yes" {
			return errors.New("workstation cleanup requires --yes")
		}
		target, err := effectiveTarget(ctx)
		if err != nil {
			return err
		}
		return cleanupWorkstationConveniences(ctx, target.Name, out)
	}}
}

func workstationTrustMarker(target string) (string, error) {
	if err := deployment.ValidateTargetName(target); err != nil {
		return "", err
	}
	root, err := deployment.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "workstation-trust", target+".json"), nil
}

func setWorkstationTrustActive(target string, active bool) error {
	path, err := workstationTrustMarker(target)
	if err != nil {
		return err
	}
	return writePrivateJSON(path, struct {
		Active bool `json:"active"`
	}{active})
}

func readConnectionCA(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var profile remoteConnectionProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", err
	}
	if profile.Version != remoteConnectionProfileVersion {
		return "", errors.New("unknown connection profile version; trust preserved")
	}
	_, fingerprint, err := hosttrust.ParseCA([]byte(profile.Trust.CAPEM))
	return fingerprint, err
}

func otherConnectionReferencesCA(root, target, fingerprint string) (bool, error) {
	profiles, err := filepath.Glob(filepath.Join(root, "remote-connections", "*", "profile.json"))
	if err != nil {
		return false, err
	}
	for _, path := range profiles {
		name := filepath.Base(filepath.Dir(path))
		if name == target {
			continue
		}
		marker, err := workstationTrustMarker(name)
		if err != nil {
			return false, err
		}
		body, err := os.ReadFile(marker)
		if err == nil {
			var state struct {
				Active bool `json:"active"`
			}
			if err := json.Unmarshal(body, &state); err != nil {
				return false, err
			}
			if !state.Active {
				continue
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		candidate, err := readConnectionCA(path)
		if err != nil {
			return false, err
		}
		if candidate == fingerprint {
			return true, nil
		}
	}
	return false, nil
}

func cleanupWorkstationConveniences(ctx context.Context, target string, out io.Writer) error {
	root, err := deployment.DataRoot()
	if err != nil {
		return err
	}
	profile := filepath.Join(root, "remote-connections", target, "profile.json")
	fingerprint, err := readConnectionCA(profile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	shared := false
	if fingerprint != "" {
		shared, err = otherConnectionReferencesCA(root, target, fingerprint)
		if err != nil {
			return err
		}
	}
	if err := removeWorkstationHosts(ctx, target, out); err != nil {
		return err
	}
	if fingerprint != "" && !shared {
		stateDir, err := bhruntime.DataDir("")
		if err != nil {
			return err
		}
		removed, err := hosttrust.RemoveOwnedFingerprint(ctx, stateDir, fingerprint)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "[OK] Removed %d owned system/browser CA anchor(s).\n", len(removed.Removed))
	} else if shared {
		fmt.Fprintln(out, "[INFO] CA retained: another workstation connection still references it.")
	}
	return setWorkstationTrustActive(target, false)
}
