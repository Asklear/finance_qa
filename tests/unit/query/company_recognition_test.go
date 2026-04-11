package query_test

import (
	"financeqa/internal/query"
	"testing"
)

func TestResolveCompanyInSentences(t *testing.T) {
	available := []string{
		"南京优集数据科技有限公司",
		"南京林悦智能科技有限公司",
	}

	tests := []struct {
		name     string
		question string
		expected string
	}{
		{"Full Name", "查询南京优集数据科技有限公司的收入", "南京优集数据科技有限公司"},
		{"Nickname Youji", "优集这月支出多少", "南京优集数据科技有限公司"},
		{"Nickname Linyue", "帮我查下林悦智能的应收", "南京林悦智能科技有限公司"},
		{"Shortest ID Linyue", "林悦最近有报销吗", "南京林悦智能科技有限公司"},
		{"Default Fallback", "今天天气不错", "南京优集数据科技有限公司"}, // Should return first available
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := query.ResolveCompany(tt.question, available)
			if got != tt.expected {
				t.Errorf("ResolveCompany() = %v, want %v", got, tt.expected)
			}
		})
	}
}
