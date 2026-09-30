package resourcepreflight

import "fmt"

const (
	MiB = uint64(1024 * 1024)
	GiB = uint64(1024 * 1024 * 1024)
)

type Confidence string

const (
	ConfidenceExplicit      Confidence = "EXPLICIT"
	ConfidenceKnownBaseline Confidence = "KNOWN_BASELINE"
	ConfidenceEstimated     Confidence = "ESTIMATED"
	ConfidenceUnknown       Confidence = "UNKNOWN"
)

type Decision string

const (
	DecisionSafe   Decision = "SAFE"
	DecisionTight  Decision = "TIGHT"
	DecisionUnsafe Decision = "UNSAFE"
)

type Requirement struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind,omitempty"`
	Scope          string     `json:"scope,omitempty"`
	MinimumBytes   uint64     `json:"minimum_bytes,omitempty"`
	EstimatedBytes uint64     `json:"estimated_bytes,omitempty"`
	Confidence     Confidence `json:"confidence"`
	Source         string     `json:"source,omitempty"`
	AlreadyRunning bool       `json:"already_running,omitempty"`
}

type Assessment struct {
	ProblemCode          string        `json:"problem_code,omitempty"`
	Host                 HostMemory    `json:"host"`
	Requirements         []Requirement `json:"requirements,omitempty"`
	RequiredMinimumBytes uint64        `json:"required_minimum_bytes"`
	EstimatedBytes       uint64        `json:"estimated_bytes"`
	SafetyMarginBytes    uint64        `json:"safety_margin_bytes"`
	UnknownCount         int           `json:"unknown_count"`
	Decision             Decision      `json:"decision"`
	Reasons              []string      `json:"reasons,omitempty"`
	MutationPerformed    bool          `json:"mutation_performed"`
	OverrideUsed         bool          `json:"override_used,omitempty"`
}

func SafetyMargin(total uint64) uint64 {
	margin := total / 10
	if margin < 512*MiB {
		margin = 512 * MiB
	}
	if margin > 2*GiB {
		margin = 2 * GiB
	}
	return margin
}

func Assess(host HostMemory, requirements []Requirement) Assessment {
	out := Assessment{
		Host: host,
		Requirements: append([]Requirement(nil), requirements...),
		SafetyMarginBytes: SafetyMargin(host.TotalBytes),
		Decision: DecisionSafe,
		MutationPerformed: false,
	}
	for _, r := range requirements {
		if r.AlreadyRunning {
			continue
		}
		switch r.Confidence {
		case ConfidenceExplicit, ConfidenceKnownBaseline:
			out.RequiredMinimumBytes += r.MinimumBytes
		case ConfidenceEstimated:
			// Estimated requirements inform headroom but never create a hard lower bound.
		case ConfidenceUnknown:
			out.UnknownCount++
		}
		estimate := r.EstimatedBytes
		if estimate < r.MinimumBytes {
			estimate = r.MinimumBytes
		}
		out.EstimatedBytes += estimate
	}
	if out.RequiredMinimumBytes > host.AvailableBytes {
		out.Decision = DecisionUnsafe
		out.ProblemCode = "host_memory_insufficient"
		out.Reasons = append(out.Reasons, fmt.Sprintf("reliable minimum %d exceeds MemAvailable %d", out.RequiredMinimumBytes, host.AvailableBytes))
		return out
	}

	if out.EstimatedBytes > 0 && out.EstimatedBytes+out.SafetyMarginBytes > host.AvailableBytes {
		out.Decision = DecisionTight
		out.ProblemCode = "host_memory_tight"
		out.Reasons = append(out.Reasons, "planned estimate plus centralized safety headroom exceeds MemAvailable")
	}

	if host.SwapTotalBytes > 0 {
		swapUsed := host.SwapTotalBytes - host.SwapFreeBytes
		if swapUsed*100 >= host.SwapTotalBytes*90 && host.AvailableBytes < out.EstimatedBytes+2*out.SafetyMarginBytes {
			out.Decision = DecisionTight
			out.ProblemCode = "host_memory_pressured"
			out.Reasons = append(out.Reasons, "swap is at least 90% used while memory headroom is narrow")
		}
	}
	if host.Pressure != nil && (host.Pressure.SomeAvg60 >= 1.0 || host.Pressure.FullAvg60 >= 0.10) {
		out.Decision = DecisionTight
		out.ProblemCode = "host_memory_pressured"
		out.Reasons = append(out.Reasons, "sustained Linux memory PSI indicates pressure")
	}
	if out.UnknownCount > 0 {
		out.Reasons = append(out.Reasons, fmt.Sprintf("%d workload requirement(s) remain UNKNOWN and are not used for hard failure", out.UnknownCount))
	}
	return out
}
