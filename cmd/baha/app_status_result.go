package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/application"
)

type applicationStatusResult struct {
	application.StatusResult
	TLS *applicationTLSObservation `json:"tls,omitempty"`

	tlsStatus *applicationTLSStatus
	tlsErr    error
}

func collectApplicationStatusResult(ctx context.Context, store application.Store, args []string) (applicationStatusResult, error) {
	result, err := collectApplicationStatus(ctx, store, args)
	if err != nil {
		return applicationStatusResult{}, err
	}
	resolved, err := resolveApplication(store, args, "status")
	if err != nil {
		return applicationStatusResult{}, err
	}
	tlsStatus, tlsObservation, tlsErr := collectApplicationTLSObservation(resolved)
	if tlsErr != nil || (tlsObservation != nil && !tlsObservation.Healthy) {
		result.Ready = false
	}
	return applicationStatusResult{
		StatusResult: result,
		TLS:          tlsObservation,
		tlsStatus:    tlsStatus,
		tlsErr:       tlsErr,
	}, nil
}
