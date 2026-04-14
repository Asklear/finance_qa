package query_test

import (
	"encoding/json"
	"testing"
	"time"

	"financeqa/internal/query"
)

func TestExtractPeriodWithNow(t *testing.T) {
	now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		question string
		wantFrom string
		wantTo   string
	}{
		{name: "explicit year month", question: "2026年2月收入是多少", wantFrom: "2026-02", wantTo: "2026-02"},
		{name: "range", question: "2026年1月到2026年2月支出", wantFrom: "2026-01", wantTo: "2026-02"},
		{name: "month only current year", question: "2月收入", wantFrom: "2026-02", wantTo: "2026-02"},
		{name: "month only previous year", question: "12月收入", wantFrom: "2025-12", wantTo: "2025-12"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := query.ExtractPeriodWithNow(tc.question, now)
			if from != tc.wantFrom || to != tc.wantTo {
				t.Fatalf("ExtractPeriodWithNow(%q) = (%s,%s), want (%s,%s)", tc.question, from, to, tc.wantFrom, tc.wantTo)
			}
		})
	}
}

func TestResolveCompany(t *testing.T) {
	available := []string{"模拟财务科技有限公司", "模拟财务", "苏州示例"}
	got := query.ResolveCompany("模拟财务", available)
	if got != "模拟财务科技有限公司" {
		t.Fatalf("ResolveCompany() = %q, want %q", got, "模拟财务科技有限公司")
	}
}

func TestClassifyIntent(t *testing.T) {
	cases := []struct {
		question string
		want     query.Intent
	}{
		{question: "2月收入是多少", want: query.IntentMonthlySummary},
		{question: "2月增值税是多少", want: query.IntentTaxQuery},
		{question: "2月应收账款情况", want: query.IntentARAPQuery},
		{question: "2月账龄分析", want: query.IntentAnalysis},
		{question: "供应商多少", want: query.IntentFallback},
		{question: "公司财务健康度怎么", want: query.IntentFallback},
		{question: "货币资金余额是多少", want: query.IntentPrecise},
	}

	for _, tc := range cases {
		if got := query.ClassifyIntent(tc.question); got != tc.want {
			t.Fatalf("ClassifyIntent(%q) = %q, want %q", tc.question, got, tc.want)
		}
	}
}

func TestClassifyIntentV2TraceContract(t *testing.T) {
	intent, tr := query.ClassifyIntentV2("汇智在2026年2月这笔是供应商付款还是预收款？")
	if intent == "" {
		t.Fatalf("expected non-empty intent")
	}

	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}

	for _, k := range []string{"router_version", "matched", "scores", "final_intent", "confidence"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing trace key=%s", k)
		}
	}
	if tr.FinalIntent != string(intent) {
		t.Fatalf("trace final_intent=%q does not match returned intent=%q", tr.FinalIntent, intent)
	}
	if tr.Confidence < 0 || tr.Confidence > 1 {
		t.Fatalf("unexpected confidence=%f", tr.Confidence)
	}
	if len(tr.Scores) == 0 {
		t.Fatalf("expected non-empty scores")
	}
}
