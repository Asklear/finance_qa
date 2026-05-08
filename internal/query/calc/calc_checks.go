package calc

import (
	"fmt"
	"math"
)

// ArithmeticCheckResult 是算术一致性守卫的统一返回结构。
type ArithmeticCheckResult struct {
	Passed  bool    `json:"passed"`
	Diff    float64 `json:"diff"`
	Message string  `json:"message"`
}

const defaultArithmeticEpsilon = 0.01

// CheckSumEqualsTotal 验证 items 之和是否与 total 一致。
// epsilon 可选，不传时默认使用 0.01。
func CheckSumEqualsTotal(items []float64, total float64, epsilon ...float64) ArithmeticCheckResult {
	sum := 0.0
	for _, item := range items {
		sum += item
	}

	diff := sum - total
	limit := resolveArithmeticEpsilon(epsilon...)
	passed := math.Abs(diff) <= limit

	return ArithmeticCheckResult{
		Passed:  passed,
		Diff:    diff,
		Message: fmt.Sprintf("sum(items)=%.2f total=%.2f diff=%.2f epsilon=%.2f", sum, total, diff, limit),
	}
}

// CheckOpeningDeltaClosing 验证 opening + delta 是否与 closing 一致。
// epsilon 可选，不传时默认使用 0.01。
func CheckOpeningDeltaClosing(opening, delta, closing float64, epsilon ...float64) ArithmeticCheckResult {
	calculated := opening + delta
	diff := math.Abs(calculated - closing)
	limit := resolveArithmeticEpsilon(epsilon...)
	passed := diff <= limit

	return ArithmeticCheckResult{
		Passed:  passed,
		Diff:    diff,
		Message: fmt.Sprintf("opening+delta=%.2f closing=%.2f diff=%.2f epsilon=%.2f", calculated, closing, diff, limit),
	}
}

func resolveArithmeticEpsilon(epsilon ...float64) float64 {
	if len(epsilon) == 0 {
		return defaultArithmeticEpsilon
	}
	if epsilon[0] <= 0 || math.IsNaN(epsilon[0]) || math.IsInf(epsilon[0], 0) {
		return defaultArithmeticEpsilon
	}
	return epsilon[0]
}
