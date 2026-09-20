package application

// StatusCheck is a secret-safe readiness observation shared by CLI and future
// machine-oriented control surfaces.
type StatusCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// StatusResult is the shared application readiness model. It intentionally
// contains operational state only and never credential or secret values.
type StatusResult struct {
	Application string        `json:"application"`
	Environment string        `json:"environment"`
	Manifest    string        `json:"manifest,omitempty"`
	Project     string        `json:"project"`
	State       string        `json:"state"`
	Ready       bool          `json:"ready"`
	Checks      []StatusCheck `json:"checks"`
}

// AddCheck appends one readiness observation and folds failures into Ready.
func (r *StatusResult) AddCheck(name string, ok bool, detail string) {
	r.Checks = append(r.Checks, StatusCheck{Name: name, OK: ok, Detail: detail})
	if !ok {
		r.Ready = false
	}
}
