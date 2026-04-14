package query_test

import (
	"math"
	"strings"
	"testing"

	"financeqa/internal/query"
)

func TestCheckSumEqualsTotalUsesDefaultEpsilon(t *testing.T) {
	got := query.CheckSumEqualsTotal([]float64{10, 20, 30}, 60.009)
	if !got.Passed {
		t.Fatalf("expected sum check to pass with default epsilon, got %+v", got)
	}
	if math.Abs(got.Diff+0.009) > 1e-9 {
		t.Fatalf("diff = %.12f, want %.12f", got.Diff, -0.009)
	}
	if !strings.Contains(got.Message, "sum(items)") {
		t.Fatalf("message should describe the sum formula, got %q", got.Message)
	}
}

func TestCheckSumEqualsTotalAllowsCustomEpsilon(t *testing.T) {
	got := query.CheckSumEqualsTotal([]float64{10, 20, 30}, 60.03, 0.05)
	if !got.Passed {
		t.Fatalf("expected sum check to pass with custom epsilon, got %+v", got)
	}
	if math.Abs(got.Diff+0.03) > 1e-9 {
		t.Fatalf("diff = %.12f, want %.12f", got.Diff, -0.03)
	}
}

func TestCheckOpeningDeltaClosingUsesDefaultEpsilon(t *testing.T) {
	got := query.CheckOpeningDeltaClosing(100, 25, 124.995)
	if !got.Passed {
		t.Fatalf("expected opening/delta/closing check to pass with default epsilon, got %+v", got)
	}
	if math.Abs(got.Diff-0.005) > 1e-9 {
		t.Fatalf("diff = %.12f, want %.12f", got.Diff, 0.005)
	}
	if !strings.Contains(got.Message, "opening+delta") {
		t.Fatalf("message should describe the opening/delta formula, got %q", got.Message)
	}
}

func TestCheckOpeningDeltaClosingAllowsCustomEpsilon(t *testing.T) {
	got := query.CheckOpeningDeltaClosing(100, 25, 125.03, 0.05)
	if !got.Passed {
		t.Fatalf("expected opening/delta/closing check to pass with custom epsilon, got %+v", got)
	}
	if math.Abs(got.Diff-0.03) > 1e-9 {
		t.Fatalf("diff = %.12f, want %.12f", got.Diff, 0.03)
	}
}
