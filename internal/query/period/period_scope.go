package period

import "strings"

func resolveRelativeHalfRange(anchorYear, anchorMonth int, label string) (string, string) {
	targetYear := anchorYear
	targetHalf := 1
	if strings.Contains(label, "下") {
		targetHalf = 2
		if anchorMonth < 7 {
			targetYear = anchorYear - 1
		}
	}
	return halfRange(targetYear, targetHalf)
}

func resolveRelativeQuarterRange(anchorYear, anchorMonth int, token string) (string, string) {
	quarter := parseQuarterToken(token)
	targetYear := anchorYear
	_, to := quarterRange(anchorYear, quarter)
	toMonth := mustAtoi(strings.Split(to, "-")[1])
	if toMonth > anchorMonth && (toMonth-anchorMonth) >= 6 {
		targetYear = anchorYear - 1
	}
	return quarterRange(targetYear, quarter)
}
