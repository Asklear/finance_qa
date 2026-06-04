package query

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEngineWithAsOfAnchorResolvesRelativePeriods(t *testing.T) {
	engine, err := NewEngine(
		filepath.Join(t.TempDir(), "as-of.sqlite"),
		"测试公司",
		WithAsOfAnchor(time.Date(2026, time.April, 14, 0, 0, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	ctx := engine.prepareQueryExecutionContext("上个月收入多少?")
	if ctx.anchor.Format("2006-01-02") != "2026-04-14" {
		t.Fatalf("anchor = %s, want 2026-04-14", ctx.anchor.Format("2006-01-02"))
	}
	if ctx.spec.AsOf != "2026-04-14" {
		t.Fatalf("QuerySpec.AsOf = %q, want 2026-04-14", ctx.spec.AsOf)
	}
	if ctx.spec.PeriodFrom != "2026-03" || ctx.spec.PeriodTo != "2026-03" {
		t.Fatalf("period = %s~%s, want 2026-03~2026-03", ctx.spec.PeriodFrom, ctx.spec.PeriodTo)
	}
}
