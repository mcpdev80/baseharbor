package machine

import "time"

// TerminalEvent carries binary PTY output as JSON base64; replay is forbidden.
type TerminalEvent struct {
	ContractVersion string    `json:"contract_version"`
	StreamID        string    `json:"stream_id"`
	Sequence        uint64    `json:"sequence"`
	Kind            string    `json:"kind"`
	OccurredAt      time.Time `json:"occurred_at"`
	Data            []byte    `json:"data,omitempty"`
	ExitCode        *int      `json:"exit_code,omitempty"`
}

type TerminalInput struct {
	ContractVersion string `json:"contract_version"`
	Sequence        uint64 `json:"sequence"`
	Kind            string `json:"kind"`
	Data            []byte `json:"data,omitempty"`
	Rows            int    `json:"rows,omitempty"`
	Columns         int    `json:"columns,omitempty"`
}
