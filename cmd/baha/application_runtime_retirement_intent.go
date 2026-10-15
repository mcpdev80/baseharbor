package main

import "github.com/mcpdev80/baseharbor/internal/application"

// Withdrawal changes requested runtime, never its canonical deployment owner.
func applicationRuntimeRetirementIntent(m application.Manifest) application.Manifest {
	return application.Manifest{Version: m.Version, ApplicationID: m.ApplicationID, Name: m.Name, Environment: m.Environment}
}
