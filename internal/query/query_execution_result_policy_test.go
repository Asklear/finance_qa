package query

import "testing"

func TestShouldFallbackExecutionResult(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "success_keeps_result",
			result: Result{Success: true, Message: "ok"},
			want:   false,
		},
		{
			name:   "account_not_found_falls_back",
			result: Result{Message: "account not found"},
			want:   true,
		},
		{
			name:   "ambiguity_falls_back",
			result: Result{Message: "语义模糊，需要更多信息"},
			want:   true,
		},
		{
			name:   "empty_message_falls_back",
			result: Result{},
			want:   true,
		},
		{
			name:   "explicit_failure_message_keeps_result",
			result: Result{Message: "db timeout"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackExecutionResult(tc.result); got != tc.want {
				t.Fatalf("shouldFallbackExecutionResult(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
