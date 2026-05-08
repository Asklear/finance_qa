package company

import "testing"

func TestBestCompanyMatchPrefersLongerFormalNameOnAliasTie(t *testing.T) {
	companies := []string{
		"南京优集",
		"南京优集数据科技有限公司",
		"苏州示例科技有限公司",
	}

	got, score := BestMatch("优集", companies)

	if got != "南京优集数据科技有限公司" {
		t.Fatalf("bestCompanyMatch() company = %q, want %q", got, "南京优集数据科技有限公司")
	}
	if score <= 0 {
		t.Fatalf("bestCompanyMatch() score = %d, want positive match score", score)
	}
}
