package result

type Result struct {
	Success         bool           `json:"success"`
	Data            map[string]any `json:"data"`
	Message         string         `json:"message"`
	AnswerMethod    string         `json:"answer_method,omitempty"`
	ExecutedSQL     []string       `json:"executed_sql"`
	CalculationLogs []string       `json:"calculation_logs"`
}

func (r Result) WithTraceData() Result {
	if len(r.ExecutedSQL) == 0 {
		r.ExecutedSQL = []string{"(trace-sql) no explicit SQL captured in this branch"}
	}
	if len(r.CalculationLogs) == 0 {
		r.CalculationLogs = []string{"(trace-log) no explicit calculation logs captured in this branch"}
	}
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	if r.AnswerMethod == "" {
		r.AnswerMethod = "sql"
	}
	executed := append([]string{}, r.ExecutedSQL...)
	logs := append([]string{}, r.CalculationLogs...)
	process := map[string]any{
		"answer_method":    r.AnswerMethod,
		"executed_sql":     executed,
		"calculation_logs": logs,
	}
	r.Data["answer_method"] = r.AnswerMethod
	r.Data["trace"] = process
	r.Data["process"] = process
	r.Data["executed_sql"] = executed
	r.Data["calculation_logs"] = logs
	return r
}
