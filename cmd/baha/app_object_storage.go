package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func requiresRuntimeObjectStorageExecutor(m application.Manifest) bool {
	return application.HasRuntimeCapabilityPermission(m, string(capability.ObjectStorageS3V1.ID))
}

func requiresObjectStorageProviderAdmin(m application.Manifest) bool {
	return application.HasObjectStorage(m) || requiresRuntimeObjectStorageExecutor(m)
}

type managedObjectStorageExecution struct {
	execution      *capability.Execution
	driver         *objectstorage.Driver
	manifest       application.Manifest
	runtimeEnabled bool
}

func prepareManagedObjectStorage(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedObjectStorageExecution, error) {
	m := resolved.Manifest
	runtimeEnabled := application.HasRuntimeCapabilityPermission(m, string(capability.ObjectStorageS3V1.ID))
	if !application.HasObjectStorage(m) && !runtimeEnabled {
		return nil, nil
	}
	files := application.RuntimeFilesFor(resolved.Store, m)
	driver := objectstorage.NewDriver(compose, m, files)
	requests := make([]capability.Request, 0, len(application.ObjectStorageBucketNames(m)))
	for _, bucket := range application.ObjectStorageBucketNames(m) {
		security := application.ObjectStorageSecureBinding(m, bucket)
		requests = append(requests, capability.Request{
			Requirement:     capability.Requirement{Kind: capability.ObjectStorageS3, Name: bucket},
			Workload:        "application/" + m.Name,
			ObjectStorageS3: &capability.ObjectStorageS3Binding{Bucket: bucket},
			Security:        &security,
			Driver:          driver,
		})
	}
	var execution *capability.Execution
	if len(requests) > 0 {
		var err error
		execution, _, err = capability.Prepare(ctx, m.Name, requests)
		if err != nil {
			return nil, err
		}
	}
	return &managedObjectStorageExecution{execution: execution, driver: driver, manifest: m, runtimeEnabled: runtimeEnabled}, nil
}

func convergeManagedObjectStorage(ctx context.Context, out io.Writer, prepared *managedObjectStorageExecution) error {
	if prepared == nil {
		return nil
	}
	if prepared.execution != nil {
		if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
			prepared.driver.Rollback(context.WithoutCancel(ctx))
			return err
		}
		if _, err := prepared.execution.Verify(ctx); err != nil {
			prepared.driver.Rollback(context.WithoutCancel(ctx))
			return err
		}
	} else if prepared.runtimeEnabled {
		if _, _, _, err := prepared.driver.EnsureSharedProvider(ctx); err != nil {
			return err
		}
	}
	if prepared.runtimeEnabled {
		fmt.Fprintln(out, "[READY] object-storage    provider and IAM ready")
	}
	for _, bucket := range application.ObjectStorageBucketNames(prepared.manifest) {
		fmt.Fprintf(out, "[VERIFIED] object-storage %s authenticated S3 Put/Get\n", bucket)
	}
	return nil
}
