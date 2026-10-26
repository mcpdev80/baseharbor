package targetsession

import "time"

type StreamOpen struct {
	ContractVersion string           `json:"contract_version"`
	ProtocolVersion string           `json:"protocol_version"`
	StreamID        string           `json:"stream_id"`
	CorrelationID   string           `json:"correlation_id"`
	TargetID        string           `json:"target_id"`
	ResourceID      string           `json:"resource_id"`
	Kind            string           `json:"kind"`
	DeadlineAt      time.Time        `json:"deadline_at"`
	Logs            *LogOptions      `json:"logs,omitempty"`
	Terminal        *TerminalOptions `json:"terminal,omitempty"`
}

type LogOptions struct {
	Tail   int    `json:"tail,omitempty"`
	Since  string `json:"since,omitempty"`
	Follow bool   `json:"follow,omitempty"`
}

type TerminalOptions struct {
	Rows int      `json:"rows"`
	Cols int      `json:"cols"`
	Argv []string `json:"argv"`
}

type streamEvent struct {
	ContractVersion string    `json:"contract_version"`
	ProtocolVersion string    `json:"protocol_version"`
	StreamID        string    `json:"stream_id"`
	CorrelationID   string    `json:"correlation_id"`
	Sequence        uint64    `json:"sequence"`
	ObservedAt      time.Time `json:"observed_at"`
	Type            string    `json:"type"`
	Data            []byte    `json:"data,omitempty"`
	Rows            int       `json:"rows,omitempty"`
	Cols            int       `json:"cols,omitempty"`
	ExitCode        *int      `json:"exit_code,omitempty"`
	Message         string    `json:"message,omitempty"`
}
