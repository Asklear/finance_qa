package query

// Variable describes one named input value used by a calc plan.
type Variable struct {
	Name        string `json:"name"`
	Value       any    `json:"value"`
	Unit        string `json:"unit,omitempty"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

// Formula describes one expression that produces an output value.
type Formula struct {
	Name        string `json:"name"`
	Expression  string `json:"expression"`
	Output      string `json:"output,omitempty"`
	Description string `json:"description,omitempty"`
}

// Check describes a rule that must pass during plan execution.
type Check struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Severity   string `json:"severity,omitempty"`
	Message    string `json:"message,omitempty"`
}

// ExecutionResult reports the outcome of executing a calc plan.
type ExecutionResult struct {
	Success      bool           `json:"success"`
	Message      string         `json:"message,omitempty"`
	Outputs      map[string]any `json:"outputs,omitempty"`
	FailedChecks []string       `json:"failed_checks,omitempty"`
	Trace        []string       `json:"trace,omitempty"`
}
