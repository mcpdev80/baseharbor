package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	"github.com/mcpdev80/baseharbor/internal/preflight"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
	"github.com/mcpdev80/baseharbor/internal/telemetry"
)

const (
	applicationStatusTimeout         = 120 * time.Second
	applicationOpenBaoStatusTimeout  = 30 * time.Second
	applicationRequiredSecretTimeout = 40 * time.Second
	applicationBrokerStatusTimeout   = 10 * time.Second
	applicationPostgresStatusTimeout = 15 * time.Second
	applicationValkeyStatusTimeout   = 10 * time.Second
	applicationLogsStatusTimeout     = 10 * time.Second
)

func collectApplicationStatus(ctx context.Context, store application.Store, args []string) (application.StatusResult, error) {
	statusCtx, cancelStatus := context.WithTimeout(ctx, applicationStatusTimeout)
	defer cancelStatus()

	collection, done, err := newApplicationStatusCollection(statusCtx, store, args)
	if err != nil || done {
		return collection.result, err
	}
	if collection.componentsStopped(statusCtx) {
		collection.result.State = "stopped"
		collection.result.Ready = false
		return collection.result, nil
	}
	collection.collectManagedServiceChecks(statusCtx)
	collection.collectWorkloadChecks()
	collection.collectLogsCheck(statusCtx)
	collection.collectExposureCheck(statusCtx)
	return collection.result, nil
}

func applicationComponentsStopped(managedRunning, workloadRunning []string, workloadFound, brokerRunning, exposureRunning bool) bool {
	if len(managedRunning) != 0 || brokerRunning || exposureRunning {
		return false
	}
	if workloadFound && len(workloadRunning) != 0 {
		return false
	}
	return true
}
