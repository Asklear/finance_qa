package query

import "testing"

func TestRankCounterpartyAliasMatchesPrefersExactThenShorterCanonicalMatches(t *testing.T) {
	got := rankCounterpartyAliasMatches("汇智", []string{
		"南京汇智互娱教育科技有限公司",
		"汇智",
		"汇智教育",
		"南京优集数据科技有限公司",
	})

	if len(got) != 3 {
		t.Fatalf("rankCounterpartyAliasMatches() len = %d, want 3", len(got))
	}
	if got[0] != "汇智" {
		t.Fatalf("rankCounterpartyAliasMatches()[0] = %q, want %q", got[0], "汇智")
	}
	if got[1] != "汇智教育" {
		t.Fatalf("rankCounterpartyAliasMatches()[1] = %q, want %q", got[1], "汇智教育")
	}
	if got[2] != "南京汇智互娱教育科技有限公司" {
		t.Fatalf("rankCounterpartyAliasMatches()[2] = %q, want %q", got[2], "南京汇智互娱教育科技有限公司")
	}
}
