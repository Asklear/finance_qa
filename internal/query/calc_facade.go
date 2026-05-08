package query

import querycalc "financeqa/internal/query/calc"

var (
	ErrMissingVariable   = querycalc.ErrMissingVariable
	ErrInvalidExpression = querycalc.ErrInvalidExpression
	ErrDivisionByZero    = querycalc.ErrDivisionByZero
)

type ArithmeticCheckResult = querycalc.ArithmeticCheckResult
type CalcExecutor = querycalc.CalcExecutor
type Variable = querycalc.Variable
type Formula = querycalc.Formula
type Check = querycalc.Check
type ExecutionResult = querycalc.ExecutionResult

func CheckSumEqualsTotal(items []float64, total float64, epsilon ...float64) ArithmeticCheckResult {
	return querycalc.CheckSumEqualsTotal(items, total, epsilon...)
}

func CheckOpeningDeltaClosing(opening, delta, closing float64, epsilon ...float64) ArithmeticCheckResult {
	return querycalc.CheckOpeningDeltaClosing(opening, delta, closing, epsilon...)
}

func NewCalcExecutor() *CalcExecutor {
	return querycalc.NewCalcExecutor()
}
