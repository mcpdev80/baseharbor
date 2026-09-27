= nil {
			c.result.AddCheck("workload", false, c.workloadErr.Error())
		} else {
			c.result.AddCheck("workload", c.workloadStatus.Ready(), fmt.Sprintf("%d/%d selected Compose service(s) ready", c.workloadStatus.ReadyCount(), len(c.workloadStatus.Services)))
		}
		return
	}
	if c.workloadErr != nil {
		c.result.AddCheck("workload", false, "repository Compose integration could not be resolved: "+c.workloadErr.Error())
	}
}

func (c *applicationStatusCollection) collectLogsCheck(ctx context.Context) {
	if !application.HasLogsCollection(c.manifest) {
		return
	}
	policy, policyErr := application.LogsPolicy(c.manifest)
	if policyErr != nil {
		c.result.AddCheck("logs", false, policyErr.Error())
		return
	}
	if !policy.Enabled || !policy.Collect[application.LogsSourceApplication] || !c.workloadStatus.Found {
		return
	}

	logServices := make([]string, 0, len(c.workloadStatus.Services))
	for _, service := range c.workloadStatus.Services {
		logServices = append(logServices, service.Service)
	}
	checkCtx, cancel := context.WithTimeout(ctx, applicationLogsStatusTimeout)
	err := logsprovider.VerifyApplicationAt(checkCtx, c.manifest, logServices, c.resolved.TargetStateRoot, c.resolved.Target.Name)
	cancel()
	if err != nil {
		c.result.AddCheck("logs", false, err.Error())
		return
	}
	c.result.AddCheck("logs", true, fmt.Sprintf("%d workload log stream(s) queryable", len(logServices)))
}

func (c *applicationStatusCollection) collectExposureCheck(ctx context.Context) {
	if len(c.manifest.Exposures) == 0 {
		return
	}
	_, exposureErr := inspectManagedExposure(ctx, c.compose, c.manifest, c.files)
	if exposureErr != nil {
		c.result.AddCheck("managed-exposure", false, exposureErr.Error())
		return
	}
	c.result.AddCheck("managed-exposure", true, "configured exposure endpoints are ready")
}
