package availability

type Health string

const (
	Healthy              Health = "healthy"
	Degraded             Health = "degraded"
	Unavailable          Health = "unavailable"
	UnsatisfiedGuarantee Health = "unsatisfied-guarantee"
)

type InstanceObservation struct {
	ID            string `json:"id"`
	Ready         bool   `json:"ready"`
	FailureDomain string `json:"failure_domain,omitempty"`
}

type ServiceObservation struct {
	Component       string                `json:"component"`
	StableEndpoints []string              `json:"stable_endpoints,omitempty"`
	Instances       []InstanceObservation `json:"instances"`
	Health          Health                `json:"health"`
	ReadyInstances  int                   `json:"ready_instances"`
}

func Observe(requirement Requirement, instances []InstanceObservation, endpoints []string) ServiceObservation {
	result := ServiceObservation{
		Component: requirement.Component,
		StableEndpoints: append([]string(nil), endpoints...),
		Instances: append([]InstanceObservation(nil), instances...),
	}
	for _, instance := range instances {
		if instance.Ready {
			result.ReadyInstances++
		}
	}
	switch {
	case len(instances) == 0 || result.ReadyInstances == 0:
		result.Health = Unavailable
	case requirement.HA && requirement.Instances > 0 && result.ReadyInstances < requirement.Instances:
		result.Health = UnsatisfiedGuarantee
	case requirement.HA && requirement.Instances == 0 && result.ReadyInstances < 2:
		result.Health = UnsatisfiedGuarantee
	case result.ReadyInstances < len(instances):
		result.Health = Degraded
	default:
		result.Health = Healthy
	}
	return result
}
