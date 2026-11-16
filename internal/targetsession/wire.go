// Package targetsession transports already selected Core operations. It does
// not choose placement, authorize actors or invoke a local runtime fallback.
package targetsession

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

const protocolVersion = "1"
const contractVersion = "baseharbor.target-access/v1"

type Node struct {
	TenantID   string `json:"tenant_id"`
	NodeID     string `json:"node_id"`
	TargetID   string `json:"target_id"`
	Runtime    string `json:"runtime"`
	Identity   string `json:"identity"`
	InstanceID string `json:"instance_id,omitempty"`
}

func (n Node) Scope() targetenrollment.Scope {
	return targetenrollment.Scope{TenantID: n.TenantID, TargetID: n.TargetID, NodeID: n.NodeID, Runtime: n.Runtime}
}

type hello struct {
	ContractVersions []string `json:"contract_versions"`
	ProtocolVersions []string `json:"protocol_versions"`
	Node             Node     `json:"node"`
}

// Request is the canonical bounded transport projection, not a host command.
type Request struct {
	ContractVersion string          `json:"contract_version"`
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	CorrelationID   string          `json:"correlation_id"`
	TargetID        string          `json:"target_id"`
	Operation       string          `json:"operation"`
	IssuedAt        time.Time       `json:"issued_at"`
	DeadlineAt      time.Time       `json:"deadline_at"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

type Response struct {
	ContractVersion string          `json:"contract_version"`
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	CorrelationID   string          `json:"correlation_id"`
	Success         bool            `json:"success"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           *Problem        `json:"error,omitempty"`
}

type Problem struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type Capability struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

type Capabilities struct {
	ContractVersion string       `json:"contract_version"`
	ProtocolVersion string       `json:"protocol_version"`
	Node            Node         `json:"node"`
	Capabilities    []Capability `json:"capabilities"`
}

func readRecord(reader io.Reader, record string, destination any) error {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > contracts.TargetAccessMaxFrameBytes {
		return contracts.ErrTargetAccessWire
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	if err := contracts.ValidateTargetAccessRecord(record, data); err != nil {
		return err
	}
	return json.Unmarshal(data, destination)
}

func writeRecord(writer io.Writer, record string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := contracts.ValidateTargetAccessRecord(record, data); err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := writer.Write(part)
			if err != nil {
				return err
			}
			if n <= 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}
