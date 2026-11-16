package targetsession

import (
	"context"
	"errors"
	"time"
)

type NodeMemoryEvidence struct {
	TotalBytes     uint64
	AvailableBytes uint64
	SwapTotalBytes uint64
	SwapFreeBytes  uint64
}

// NodeMemory requests fresh execution-node evidence over the same exact
// authenticated scope as project mutations. Core-host capacity is no fallback.
func (r *ProjectRuntime) NodeMemory(ctx context.Context) (NodeMemoryEvidence, error) {
	started := time.Now().UTC()
	var snapshot struct {
		ObservedAt time.Time `json:"observed_at"`
		Runtime    string    `json:"runtime"`
		NodeMemory *struct {
			Total     *uint64 `json:"total_bytes"`
			Available *uint64 `json:"available_bytes"`
			SwapTotal *uint64 `json:"swap_total_bytes"`
			SwapFree  *uint64 `json:"swap_free_bytes"`
		} `json:"node_memory"`
	}
	if err := r.invoke(ctx, "connector.health", struct{}{}, &snapshot); err != nil {
		return NodeMemoryEvidence{}, err
	}
	memory := snapshot.NodeMemory
	if snapshot.Runtime != r.scope.Runtime || snapshot.ObservedAt.Before(started.Add(-5*time.Second)) || snapshot.ObservedAt.After(time.Now().UTC().Add(5*time.Second)) ||
		memory == nil || memory.Total == nil || memory.Available == nil || memory.SwapTotal == nil || memory.SwapFree == nil ||
		*memory.Total == 0 || *memory.Available > *memory.Total || *memory.SwapFree > *memory.SwapTotal {
		return NodeMemoryEvidence{}, errors.New("fresh execution-node memory evidence is unavailable")
	}
	return NodeMemoryEvidence{TotalBytes: *memory.Total, AvailableBytes: *memory.Available, SwapTotalBytes: *memory.SwapTotal, SwapFreeBytes: *memory.SwapFree}, nil
}
