package model

// WorkloadStatus is the provider-neutral observation of one workload service.
// Provider-native conditions and object names remain implementation details.
type WorkloadStatus struct {
	Service string
	Ready   bool
	Running bool
	Detail  string
}

// Observation is the current provider view of an application workload.
type Observation struct {
	Found    bool
	Services []WorkloadStatus
}

func (o Observation) Ready() bool {
	if !o.Found || len(o.Services) == 0 {
		return false
	}
	for _, service := range o.Services {
		if !service.Ready {
			return false
		}
	}
	return true
}
