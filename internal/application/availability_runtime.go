package application

// managedHAMemberCount resolves runtime-only member cardinality from the
// provider-neutral availability intent. Provider/member names never enter the
// portable application contract.
func managedHAMemberCount(m Manifest, component string, recommended int) int {
	req := AvailabilityIntent(m).Resolve(component)
	if !req.HA {
		return 1
	}
	if req.Instances > 0 {
		return req.Instances
	}
	if recommended > 1 {
		return recommended
	}
	return 3
}
