package machine

import "time"

const StreamContractVersion = "v1"

type StreamKind string

const (
	StreamLogs StreamKind = "logs"
	StreamExec StreamKind = "exec"
)

type StreamRequest struct {
	ContractVersion string           `json:"contract_version"`
	Kind            StreamKind       `json:"kind"`
	Context         OperationContext `json:"context"`
	ResourceKind    string           `json:"resource_kind"`
	ResourceID      string           `json:"resource_id"`
	Since           *time.Time       `json:"since,omitempty"`
	Tail            int              `json:"tail,omitempty"`
	Follow          bool             `json:"follow,omitempty"`
	Command         []string         `json:"command,omitempty"`
	TTY             bool             `json:"tty,omitempty"`
}

type StreamDescriptor struct {
	ContractVersion string           `json:"contract_version"`
	StreamID        string           `json:"stream_id"`
	Kind            StreamKind       `json:"kind"`
	Actor           ActorRef         `json:"actor"`
	Context         OperationContext `json:"context"`
	ResourceKind    string           `json:"resource_kind"`
	ResourceID      string           `json:"resource_id"`
	CreatedAt       time.Time        `json:"created_at"`
}

func (r StreamRequest) Validate() error {
	if r.ContractVersion != StreamContractVersion {
		return NewError(ErrorValidationFailed, "Unsupported stream contract version.", "Use stream contract v1.", false)
	}
	if r.Kind != StreamLogs && r.Kind != StreamExec {
		return NewError(ErrorValidationFailed, "Unsupported stream kind.", "Use logs or exec.", false)
	}
	if r.ResourceKind == "" || r.ResourceID == "" {
		return NewError(ErrorValidationFailed, "Runtime resource kind and stable resource id are required.", "Resolve a runtime resource through the Runtime Explorer first.", false)
	}
	if r.Kind == StreamExec && len(r.Command) == 0 {
		return NewError(ErrorValidationFailed, "Exec stream requires an explicit bounded command.", "Provide a container/pod command; host shell is not implied.", false)
	}
	if r.Kind == StreamLogs && len(r.Command) != 0 {
		return NewError(ErrorValidationFailed, "Log streams do not accept commands.", "Remove command from the log stream request.", false)
	}
	return nil
}
