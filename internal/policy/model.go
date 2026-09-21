package policy

type Decision string

const (
	Allow Decision = "allow"
	Warn  Decision = "warn"
	Deny  Decision = "deny"
)

const ContractVersion = "v1"

type Source string

const (
	SourceDefault          Source = "baseharbor_default"
	SourceOperatorOverride Source = "operator_override"
	SourceWorkload         Source = "workload_security"
)

type Finding struct {
	Code     string   `json:"code"`
	Decision Decision `json:"decision"`
	Scope    string   `json:"scope"`
	Subject  string   `json:"subject,omitempty"`
	Field    string   `json:"field,omitempty"`
	Value    string   `json:"value,omitempty"`
	Message  string   `json:"message"`
	Source   Source   `json:"source"`
}

type Rule struct {
	Code        string   `json:"code"`
	Decision    Decision `json:"decision"`
	Overridable bool     `json:"overridable"`
	Description string   `json:"description"`
}

type Result struct {
	ContractVersion string    `json:"contract_version"`
	Environment     string    `json:"environment"`
	Profile         string    `json:"profile"`
	Decision        Decision  `json:"decision"`
	Findings        []Finding `json:"findings"`
	Rules           []Rule    `json:"rules,omitempty"`
	Overrides       []string  `json:"overrides,omitempty"`
}

func Aggregate(findings []Finding) Decision {
	decision := Allow
	for _, finding := range findings {
		switch finding.Decision {
		case Deny:
			return Deny
		case Warn:
			decision = Warn
		}
	}
	return decision
}

func (r Result) Denied() bool { return r.Decision == Deny }
