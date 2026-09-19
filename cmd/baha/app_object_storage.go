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

type managedObjectStorageExecution struct {
	execution *capability.Execution
	driver    *objectstorage.Driver
}

func prepareManagedObjectStorage(ctx context.Context, compose bhruntime.Compose, resolved resolvedApplication) (*managedObjectStorageExecution, error) {
	m := resolved.Manifest
	if !application.HasObjectStorage(m) {
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
	execution, _, err := capability.Prepare(ctx, m.Name, requests)
	if err != nil {
		return nil, err
	}
	return &managedObjectStorageExecution{execution: execution, driver: driver}, nil
}

func convergeManagedObjectStorage(ctx context.Context, out io.Writer, prepared *managedObjectStorageExecution) error {
	if prepared == nil {
		return nil
	}
	if _, err := prepared.execution.ProvisionAndBind(ctx); err != nil {
		prepared.driver.Rollback(context.WithoutCancel(ctx))
		return err
	}
	if _, err := prepared.execution.Verify(ctx); err != nil {
		prepared.driver.Rollback(context.WithoutCancel(ctx))
		return err
	}
	for _, bucket := range application.ObjectStorageBucketNames(prepared.driver.Manifest()) {
		fmt.Fprintf(out, "[OK] object-storage    %s authenticated S3 Put/Get succeeded\n", bucket)
	}
	return nil
}
