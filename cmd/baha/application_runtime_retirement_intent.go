package main

import "github.com/mcpdev80/baseharbor/internal/application"

// Withdrawal changes requested runtime, never its canonical deployment owner.
func applicationRuntimeRetirementIntent(m application.Manifest) application.Manifest {
	// The withdrawn projection requests no capabilities, so it also requests no
	// providers. Keep the deployment's persisted preferences intact for recovery;
	// only this detached resolver projection overrides their normal inheritance.
	retired := application.Manifest{Version: m.Version, ApplicationID: m.ApplicationID, Name: m.Name, Environment: m.Environment}
	return application.WithProviderPreferences(retired, []application.ProviderPreference{})
}
