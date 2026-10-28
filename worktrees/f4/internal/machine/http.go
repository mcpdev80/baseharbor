package machine

// HTTPBinding is the canonical browser endpoint, including named resource
// placeholders. Clients pin the Core origin and substitute validated IDs only.
type HTTPBinding struct {
	Href     string `json:"href"`
	Method   string `json:"method"`
	Protocol string `json:"protocol,omitempty"`
}

func MachineHTTPBindings() map[string]HTTPBinding {
	const root = "/api/v1/machine"
	return map[string]HTTPBinding{
		"discovery":        {Href: root + "/discovery", Method: "GET"},
		"execute":          {Href: root + "/executions", Method: "POST"},
		"execution":        {Href: root + "/executions/{execution_id}", Method: "GET"},
		"execution_events": {Href: root + "/executions/{execution_id}/events", Method: "GET", Protocol: "sse"},
		"logs":             {Href: root + "/streams/logs", Method: "POST", Protocol: "text"},
		"exec":             {Href: root + "/streams/exec", Method: "POST", Protocol: "text"},
		"terminal_open":    {Href: root + "/terminals", Method: "POST"},
		"terminal_events":  {Href: root + "/terminals/{stream_id}/events", Method: "GET", Protocol: "sse"},
		"terminal_input":   {Href: root + "/terminals/{stream_id}/input", Method: "POST"},
		"terminal_close":   {Href: root + "/terminals/{stream_id}", Method: "DELETE"},
	}
}
