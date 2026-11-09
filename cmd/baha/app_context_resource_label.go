package main

// ResourceLabel renders the canonical "target/name/environment" resource
// reference used across machine errors, JSON results and human output. It is
// shared by every app command so the dangling-record command reports the exact
// same resource id the registry uses.
func (r resolvedApplication) ResourceLabel() string {
	if r.DeploymentRecord != nil {
		return r.DeploymentRecord.Identity.Target + "/" + r.DeploymentRecord.Identity.Application + "/" + r.DeploymentRecord.Identity.Environment
	}
	return r.DeploymentIdentity.Target + "/" + r.DeploymentIdentity.Application + "/" + r.DeploymentIdentity.Environment
}

