package main

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestBuildWorkloadServiceStatuses(t *testing.T) {
	statuses := buildWorkloadServiceStatuses(
		[]string{"worker", "edge", "api", "web"},
		[]bhruntime.ServiceState{
			{Service: "api", State: "running", Health: "healthy"},
			{Service: "edge", State: "running"},
			{Service: "web", State: "running", Health: "starting"},
		},
	)
	if len(statuses) != 4 {
		t.Fatalf("got %d statuses, want 4", len(statuses))
	}
	if statuses[0].Service != "api" || !statuses[0].Ready || statuses[0].Health != "healthy" {
		t.Fatalf("unexpected api status: %#v", statuses[0])
	}
	if statuses[1].Service != "edge" || !statuses[1].Ready {
		t.Fatalf("unexpected edge status: %#v", statuses[1])
	}
	if statuses[2].Service != "web" || statuses[2].Ready || statuses[2].Health != "starting" {
		t.Fatalf("unexpected web status: %#v", statuses[2])
	}
	if statuses[3].Service != "worker" || statuses[3].Ready || statuses[3].State != "not running" {
		t.Fatalf("unexpected worker status: %#v", statuses[3])
	}
}

func TestRepositoryWorkloadStatusReady(t *testing.T) {
	status := repositoryWorkloadStatus{Found: true, Services: []workloadServiceStatus{
		{Service: "api", State: "running", Ready: true},
		{Service: "web", State: "running", Ready: true},
	}}
	if !status.Ready() || status.ReadyCount() != 2 {
		t.Fatalf("expected ready status: %#v", status)
	}
	status.Services[1].Ready = false
	if status.Ready() || status.ReadyCount() != 1 {
		t.Fatalf("expected partial failure: %#v", status)
	}
}

func TestFormatWorkloadServiceStatus(t *testing.T) {
	if got := formatWorkloadServiceStatus(workloadServiceStatus{State: "running", Health: "healthy"}); got != "running health=healthy" {
		t.Fatalf("got %q", got)
	}
	if got := formatWorkloadServiceStatus(workloadServiceStatus{State: "not running"}); got != "not running" {
		t.Fatalf("got %q", got)
	}
}
