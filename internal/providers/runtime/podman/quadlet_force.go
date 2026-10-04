package podman

import (
	"context"
	"time"
)

func quadletForceRestartProjectNoBuild(ctx context.Context, project QuadletProject, selected []string) error {
	if err := quadletInstallProject(ctx, project); err != nil {
		return err
	}
	networks, volumes, _ := quadletProjectResourceUnits(project)
	if err := quadletEnsureResourceUnits(ctx, project, networks, "network", "NetworkName"); err != nil {
		return err
	}
	if err := quadletEnsureResourceUnits(ctx, project, volumes, "volume", "VolumeName"); err != nil {
		return err
	}
	units, err := quadletProjectServiceUnits(project, selected)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	persistentUnits := quadletPersistentServiceUnits(project, units)
	if _, err := quadletSystemctl(ctx, nil, append([]string{"restart"}, units...)...); err != nil {
		if len(persistentUnits) == 0 {
			return quadletServiceStartError(ctx, units, err)
		}
		if waitErr := quadletWaitServiceUnitsActive(ctx, persistentUnits, 120*time.Second); waitErr != nil {
			return quadletServiceStartError(ctx, units, err)
		}
	} else if err := quadletWaitServiceUnitsActive(ctx, persistentUnits, 120*time.Second); err != nil {
		return err
	}
	return quadletEnsureServiceContainersExist(ctx, project, selected)
}
