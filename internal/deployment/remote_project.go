package deployment

import "fmt"

func validateRemoteProject(record DeploymentRecord) error {
	project := record.Applied.RemoteProject
	if project == nil {
		return nil
	}
	if err := project.Validate(); err != nil {
		return fmt.Errorf("invalid remote deployment binding: %w", err)
	}
	if project.Scope.TargetID != record.Identity.Target || project.Scope.Runtime != record.Applied.RuntimeProvider {
		return fmt.Errorf("remote project does not match deployment Target and runtime")
	}
	return nil
}
