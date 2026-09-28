package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestApplicationComponentsStopped(t *testing.T) {
	tests := []struct {
		name            string
		managed         []string
		workload        []string
		workloadFound   bool
		brokerRunning   bool
		exposureRunning bool
		want            bool
	}{
		{name: "fully stopped managed app", workloadFound: true, want: true},
		{name: "workload only stopped app", workloadFound: true, want: true},
		{name: "managed backend running", managed: []string{"postgres"}, workloadFound: true, want: false},
		{name: "workload running", workload: []string{"web"}, workloadFound: true, want: false},
		{name: "broker running", workloadFound: true, brokerRunning: true, want: false},
		{name: "managed exposure running", workloadFound: true, exposureRunning: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applicationComponentsStopped(test.managed, test.workload, test.workloadFound, test.brokerRunning, test.exposureRunning); got != test.want {
				t.Fatalf("applicationComponentsStopped() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestApplicationStatusCommandError(t *testing.T) {
	tests := []struct {
		name    string
		result  application.StatusResult
		wantErr bool
	}{
		{name: "ready", result: application.StatusResult{State: "running", Ready: true}},
		{name: "running unverified", result: application.StatusResult{State: "running", Ready: false, Checks: []application.StatusCheck{{Name: "workload/worker", State: "unverified"}}}},
		{name: "stopped", result: application.StatusResult{State: "stopped", Ready: false}},
		{name: "not applied", result: application.StatusResult{State: "not_applied", Ready: false}},
		{name: "failed readiness", result: application.StatusResult{State: "running", Ready: false, Checks: []application.StatusCheck{{Name: "workload/api", State: "failed"}}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := applicationStatusCommandError(test.result)
			if (err != nil) != test.wantErr {
				t.Fatalf("applicationStatusCommandError() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
