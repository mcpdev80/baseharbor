package main

import (
	"context"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/evidence"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func (e *bahaMachineExecutor) RecordTerminalAudit(ctx context.Context, descriptor machine.StreamDescriptor, outcome string) error {
	root, err := deployment.TargetStateRoot(descriptor.Context.Target)
	if err != nil {
		return err
	}
	ctx = evidence.WithActor(ctx, "http", descriptor.Actor.Subject)
	event := evidence.NewAuditEvent(ctx, descriptor.Context.Target, descriptor.Context.Application, descriptor.Context.Environment, "runtime.terminal", outcome)
	event.Actor.Issuer = descriptor.Actor.Issuer
	event.Actor.Subject = descriptor.Actor.Subject
	event.Actor.Assurance = descriptor.Actor.Assurance
	event.Actor.Methods = append([]string(nil), descriptor.Actor.Methods...)
	event.Resource = descriptor.ResourceID
	event.CorrelationID = descriptor.StreamID
	event.AuthorizationResult = "allow"
	return evidence.Append(root, event)
}
