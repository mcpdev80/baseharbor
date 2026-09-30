package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "v1"

type StateKind string

const (
	StateDesired     StateKind = "desired"
	StateEnforced    StateKind = "enforced"
	StateObserved    StateKind = "observed"
	StateVerified    StateKind = "verified"
	StateException   StateKind = "exception"
	StateUnsupported StateKind = "unsupported"
)

type Actor struct {
	Interface string   `json:"interface"`
	Identity  string   `json:"identity,omitempty"`
	Issuer    string   `json:"issuer,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Assurance string   `json:"assurance,omitempty"`
	Methods   []string `json:"authentication_methods,omitempty"`
}

type Record struct {
	Kind       StateKind `json:"kind"`
	ID         string    `json:"id"`
	Status     string    `json:"status,omitempty"`
	Resource   string    `json:"resource,omitempty"`
	Capability string    `json:"capability,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Placement  string    `json:"placement,omitempty"`
	Ownership  string    `json:"ownership,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type RecoveryContributor struct {
	StateClass         string `json:"state_class"`
	LogicalResource    string `json:"logical_resource,omitempty"`
	Ownership          string `json:"ownership"`
	Support            string `json:"support"`
	Selected           bool   `json:"selected"`
	Durable            bool   `json:"durable,omitempty"`
	ExplicitlyExcluded bool   `json:"explicitly_excluded,omitempty"`
	Verified           bool   `json:"verified,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type Recovery struct {
	LastBackup   *time.Time            `json:"last_backup,omitempty"`
	LastRestore  *time.Time            `json:"last_restore,omitempty"`
	Contributors []RecoveryContributor `json:"contributors,omitempty"`
}

type AuditEvent struct {
	SchemaVersion       string    `json:"schema_version"`
	ID                  string    `json:"id"`
	Timestamp           time.Time `json:"timestamp"`
	Actor               Actor     `json:"actor"`
	Target              string    `json:"target"`
	Application         string    `json:"application"`
	Environment         string    `json:"environment"`
	Operation           string    `json:"operation"`
	Capability          string    `json:"capability,omitempty"`
	Resource            string    `json:"resource,omitempty"`
	Provider            string    `json:"provider,omitempty"`
	Placement           string    `json:"placement,omitempty"`
	Ownership           string    `json:"ownership,omitempty"`
	PolicyResult        string    `json:"policy_result,omitempty"`
	AuthorizationResult string    `json:"authorization_result,omitempty"`
	CorrelationID       string    `json:"correlation_id,omitempty"`
	LifecycleResult     string    `json:"lifecycle_result,omitempty"`
	VerificationResult  string    `json:"verification_result,omitempty"`
	Outcome             string    `json:"outcome"`
	Detail              string    `json:"detail,omitempty"`
}

type Integrity struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

type Bundle struct {
	ContractVersion string       `json:"contract_version"`
	SchemaVersion   string       `json:"schema_version"`
	Target          string       `json:"target"`
	Application     string       `json:"application"`
	Environment     string       `json:"environment"`
	Desired         []Record     `json:"desired_state"`
	Enforced        []Record     `json:"enforced_policy"`
	Observed        []Record     `json:"observed_state"`
	Verified        []Record     `json:"verified_result"`
	Exceptions      []Record     `json:"explicit_exceptions,omitempty"`
	Unsupported     []Record     `json:"unsupported_controls,omitempty"`
	Recovery        *Recovery    `json:"recovery,omitempty"`
	Audit           []AuditEvent `json:"audit_events,omitempty"`
	Integrity       Integrity    `json:"integrity"`
}

type actorContextKey struct{}

func WithActor(ctx context.Context, interfaceName, identity string) context.Context {
	return context.WithValue(ctx, actorContextKey{}, Actor{
		Interface: strings.TrimSpace(interfaceName),
		Identity:  strings.TrimSpace(identity),
	})
}

func ActorFromContext(ctx context.Context) Actor {
	if actor, ok := ctx.Value(actorContextKey{}).(Actor); ok && strings.TrimSpace(actor.Interface) != "" {
		return actor
	}
	return Actor{Interface: "cli", Identity: "local-operator"}
}

func NewAuditEvent(ctx context.Context, target, application, environment, operation, outcome string) AuditEvent {
	event := AuditEvent{
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Now().UTC(),
		Actor:         ActorFromContext(ctx),
		Target:        strings.TrimSpace(target),
		Application:   strings.TrimSpace(application),
		Environment:   strings.TrimSpace(environment),
		Operation:     strings.TrimSpace(operation),
		Outcome:       strings.TrimSpace(outcome),
	}
	event.CorrelationID = auditCorrelationID(event)
	event.ID = auditEventID(event)
	return event
}

func auditCorrelationID(event AuditEvent) string {
	copy := event
	copy.ID = ""
	copy.CorrelationID = ""
	data, _ := json.Marshal(copy)
	sum := sha256.Sum256(append([]byte("correlation:"), data...))
	return hex.EncodeToString(sum[:12])
}

func (e AuditEvent) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return errors.New("unsupported audit schema version")
	}
	if e.ID == "" || e.Timestamp.IsZero() || e.Actor.Interface == "" || e.Target == "" || e.Application == "" || e.Environment == "" || e.Operation == "" || e.Outcome == "" {
		return errors.New("audit event is incomplete")
	}
	return nil
}

func auditEventID(event AuditEvent) string {
	copy := event
	copy.ID = ""
	data, _ := json.Marshal(copy)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

func Seal(bundle Bundle) (Bundle, error) {
	bundle.ContractVersion = SchemaVersion
	bundle.SchemaVersion = SchemaVersion
	bundle.Integrity = Integrity{}
	sortRecords(bundle.Desired)
	sortRecords(bundle.Enforced)
	sortRecords(bundle.Observed)
	sortRecords(bundle.Verified)
	sortRecords(bundle.Exceptions)
	sortRecords(bundle.Unsupported)
	sort.Slice(bundle.Audit, func(i, j int) bool {
		if bundle.Audit[i].Timestamp.Equal(bundle.Audit[j].Timestamp) {
			return bundle.Audit[i].ID < bundle.Audit[j].ID
		}
		return bundle.Audit[i].Timestamp.Before(bundle.Audit[j].Timestamp)
	})
	data, err := json.Marshal(bundle)
	if err != nil {
		return Bundle{}, err
	}
	sum := sha256.Sum256(data)
	bundle.Integrity = Integrity{Algorithm: "sha256", Digest: hex.EncodeToString(sum[:])}
	return bundle, nil
}

func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].ID == records[j].ID {
			return records[i].Resource < records[j].Resource
		}
		return records[i].ID < records[j].ID
	})
}
