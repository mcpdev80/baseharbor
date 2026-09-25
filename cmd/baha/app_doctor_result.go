package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

type applicationDoctorWorkloadResult struct {
	Service string `json:"service"`
	Ready   bool   `json:"ready"`
	Detail  string `json:"detail,omitempty"`
}

type applicationDoctorSecretResult struct {
	Name      string `json:"name"`
	Present   bool   `json:"present"`
	Usable    bool   `json:"usable"`
	Generated bool   `json:"generated,omitempty"`
}

type applicationDoctorResult struct {
	ContractVersion string                                       `json:"contract_version"`
	Target          string                                       `json:"target"`
	Application     string                                       `json:"application"`
	Environment     string                                       `json:"environment"`
	State           string                                       `json:"state"`
	Healthy         bool                                         `json:"healthy"`
	Checks          []preflight.Result                           `json:"checks"`
	Workload        []applicationDoctorWorkloadResult            `json:"workload,omitempty"`
	RequiredSecrets []applicationDoctorSecretResult              `json:"required_secrets,omitempty"`
	TLS             *applicationTLSObservation                   `json:"tls,omitempty"`
	ServiceTLS      []application.BackendTLSLifecycleObservation `json:"service_tls,omitempty"`

	manifest               application.Manifest
	workloadStatus         repositoryWorkloadStatus
	requiredSecretStatuses []openbao.RequiredSecretStatus
	workloadSecurity       application.WorkloadSecurityReport
	tlsStatus              *applicationTLSStatus
	tlsErr                 error
	serviceTLSErr          error
}

func collectApplicationDoctor(ctx context.Context, store application.Store, args []string) (applicationDoctorResult, error) {
	collector, done, err := newApplicationDoctorCollector(ctx, store, args)
	if err != nil || done {
		return collector.result, err
	}
	collector.runChecks(ctx)
	collector.collectTLS()
	collector.finalize()
	return collector.result, nil
}
