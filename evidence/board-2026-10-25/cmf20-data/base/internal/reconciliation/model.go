package reconciliation

import "fmt"

type State string

const (
	StateMissing          State = "missing"
	StateInSync           State = "in_sync"
	StateDrift            State = "drift"
	StateConflict         State = "conflict"
	StateForeignOwnership State = "foreign_ownership"
	StateUnsupported      State = "unsupported"
	StateDegraded         State = "degraded"
)

type Ownership string

const (
	OwnershipBaseHarbor Ownership = "baseharbor"
	OwnershipExternal   Ownership = "external"
	OwnershipForeign    Ownership = "foreign"
	OwnershipUnknown    Ownership = "unknown"
)

type Action string

const (
	ActionCreate  Action = "create"
	ActionNoop    Action = "noop"
	ActionRepair  Action = "repair"
	ActionDestroy Action = "destroy"
	ActionObserve Action = "observe"
	ActionBlocked Action = "blocked"
)

type Desired struct {
	Exists bool      `json:"exists"`
	Digest string    `json:"digest,omitempty"`
	Owner  Ownership `json:"owner"`
}

type Observed struct {
	Exists      bool      `json:"exists"`
	Digest      string    `json:"digest,omitempty"`
	Owner       Ownership `json:"owner"`
	Conflict    bool      `json:"conflict,omitempty"`
	Unsupported bool      `json:"unsupported,omitempty"`
	Degraded    bool      `json:"degraded,omitempty"`
	Message     string    `json:"message,omitempty"`
}

type Diff struct {
	DesiredDigest  string `json:"desired_digest,omitempty"`
	ObservedDigest string `json:"observed_digest,omitempty"`
	Changed        bool   `json:"changed"`
}

type Result struct {
	State     State     `json:"state"`
	Action    Action    `json:"action"`
	Ownership Ownership `json:"ownership"`
	Diff      Diff      `json:"diff"`
	Message   string    `json:"message,omitempty"`
}

func Evaluate(desired Desired, observed Observed) Result {
	result := Result{
		Ownership: observed.Owner,
		Diff: Diff{
			DesiredDigest:  desired.Digest,
			ObservedDigest: observed.Digest,
			Changed:        desired.Exists != observed.Exists || desired.Digest != observed.Digest,
		},
	}

	if desired.Owner == "" {
		desired.Owner = OwnershipBaseHarbor
	}
	if observed.Owner == "" {
		observed.Owner = OwnershipUnknown
		result.Ownership = observed.Owner
	}
	if observed.Unsupported {
		result.State, result.Action, result.Message = StateUnsupported, ActionBlocked, messageOr(observed.Message, "target cannot satisfy desired state")
		return result
	}
	if observed.Conflict {
		result.State, result.Action, result.Message = StateConflict, ActionBlocked, messageOr(observed.Message, "multiple reconciliation owners or conflicting state detected")
		return result
	}
	if observed.Exists && observed.Owner != OwnershipUnknown && observed.Owner != desired.Owner {
		result.State, result.Action, result.Message = StateForeignOwnership, ActionBlocked, messageOr(observed.Message, fmt.Sprintf("observed owner %q does not match expected owner %q", observed.Owner, desired.Owner))
		return result
	}
	if observed.Degraded {
		result.State, result.Action, result.Message = StateDegraded, ActionBlocked, messageOr(observed.Message, "observed resource is degraded")
		return result
	}

	if desired.Owner == OwnershipExternal {
		switch {
		case desired.Exists == observed.Exists && desired.Digest == observed.Digest:
			result.State, result.Action = StateInSync, ActionObserve
		default:
			result.State, result.Action = StateDrift, ActionObserve
		}
		return result
	}

	switch {
	case !desired.Exists && !observed.Exists:
		result.State, result.Action = StateInSync, ActionNoop
	case desired.Exists && !observed.Exists:
		result.State, result.Action = StateMissing, ActionCreate
	case !desired.Exists && observed.Exists:
		if observed.Owner == OwnershipBaseHarbor {
			result.State, result.Action = StateDrift, ActionDestroy
		} else {
			result.State, result.Action, result.Message = StateForeignOwnership, ActionBlocked, "refusing to destroy a resource not owned by BaseHarbor"
		}
	case desired.Digest == observed.Digest:
		result.State, result.Action = StateInSync, ActionNoop
	default:
		result.State, result.Action = StateDrift, ActionRepair
	}
	return result
}

func MarkVerified(result Result) Result {
	if result.State == StateDegraded {
		result.State = StateInSync
	}
	result.Message = ""
	return result
}

func MarkDegraded(result Result, message string) Result {
	result.State = StateDegraded
	result.Action = ActionBlocked
	result.Message = message
	return result
}

func messageOr(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}
