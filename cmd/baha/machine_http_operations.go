package main

import "sort"

type httpOperationKind uint8

const (
	httpRead httpOperationKind = iota + 1
	httpRuntimeMutation
	httpPlatformMutation
	httpLifecycle
)

// The executor and discovery share this transport support registry. Semantic
// descriptors, safety and authorization still come from machine.Operations.
var httpOperationKinds = map[string]httpOperationKind{
	"operator.identity":    httpRead,
	"control-plane.status": httpRead,
	"control-plane.up":     httpPlatformMutation,
	"target":               httpRead,
	"target.list":          httpRead,
	"app.list":             httpRead,
	"workspace.list":       httpRead,
	"inspect":              httpRead,
	"workspace.resolve":    httpRead,
	"workspace.status":     httpRead,
	"runtime.capabilities": httpRead,
	"runtime.list":         httpRead,
	"runtime.inspect":      httpRead,
	"runtime.metrics":      httpRead,
	"plan":                 httpRead,
	"status":               httpRead,
	"doctor":               httpRead,
	"observe":              httpRead,
	"evidence":             httpRead,
	"provider.list":        httpRead,
	"provider.inspect":     httpRead,
	"provider.verify":      httpRead,
	"organization.inspect": httpRead,
	"organization.check":   httpRead,
	"policy.check":         httpRead,
	"policy.explain":       httpRead,
	"runtime.start":        httpRuntimeMutation,
	"runtime.stop":         httpRuntimeMutation,
	"runtime.restart":      httpRuntimeMutation,
	"workspace.update":     httpPlatformMutation,
	"app.new":              httpPlatformMutation,
	"provider.add":         httpPlatformMutation,
	"provider.remove":      httpPlatformMutation,
	"organization.set":     httpPlatformMutation,
	"organization.update":  httpPlatformMutation,
	"apply":                httpLifecycle,
	"update":               httpLifecycle,
	"repair":               httpLifecycle,
	"backup":               httpLifecycle,
	"restore":              httpLifecycle,
	"destroy":              httpLifecycle,
}

func (e *bahaMachineExecutor) SupportedOperationIDs() []string {
	ids := make([]string, 0, len(httpOperationKinds))
	for id := range httpOperationKinds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
