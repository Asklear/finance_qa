package query_test

import (
	"errors"
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestCalcExecutorExecute(t *testing.T) {
	exec := query.NewCalcExecutor()

	cases := []struct {
		name string
		expr string
		vars map[string]float64
		want float64
	}{
		{
			name: "basic arithmetic with parentheses",
			expr: "a + b * (c - d) / e",
			vars: map[string]float64{
				"a": 2,
				"b": 6,
				"c": 8,
				"d": 2,
				"e": 3,
			},
			want: 14,
		},
		{
			name: "unary minus and identifier variants",
			expr: "-a + b * (_c1 - 2)",
			vars: map[string]float64{
				"a":   3,
				"b":   4,
				"_c1": 5,
			},
			want: 9,
		},
		{
			name: "nested unary minus with parentheses",
			expr: "-(a + b) + c",
			vars: map[string]float64{
				"a": 1,
				"b": 2,
				"c": 10,
			},
			want: 7,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := exec.Execute(tc.expr, tc.vars)
			if err != nil {
				t.Fatalf("Execute(%q) returned error: %v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("Execute(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestCalcExecutorExecuteMissingVariable(t *testing.T) {
	exec := query.NewCalcExecutor()

	_, err := exec.Execute("a + b", map[string]float64{"a": 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, query.ErrMissingVariable) {
		t.Fatalf("expected ErrMissingVariable, got %v", err)
	}
	if !strings.Contains(err.Error(), "b") {
		t.Fatalf("expected error to mention missing variable b, got %v", err)
	}
}

func TestCalcExecutorExecuteInvalidExpression(t *testing.T) {
	exec := query.NewCalcExecutor()

	_, err := exec.Execute("a + (b * c", map[string]float64{
		"a": 1,
		"b": 2,
		"c": 3,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, query.ErrInvalidExpression) {
		t.Fatalf("expected ErrInvalidExpression, got %v", err)
	}
}

func TestCalcExecutorExecuteDivisionByZero(t *testing.T) {
	exec := query.NewCalcExecutor()

	_, err := exec.Execute("a / (b - b)", map[string]float64{
		"a": 1,
		"b": 1,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, query.ErrDivisionByZero) {
		t.Fatalf("expected ErrDivisionByZero, got %v", err)
	}
}
