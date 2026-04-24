package query

import (
	"encoding/json"
	"strconv"
	"strings"
)

func parseRuleConfigCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func parseRuleConfigFloat(raw string) (float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseRuleConfigInt(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseRuleConfigStringSliceMap(raw string) (map[string][]string, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var v map[string][]string
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func parseRuleConfigIntMap(raw string) (map[string]int, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var v map[string]int
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}

func parseRuleConfigFloatMap(raw string) (map[string]float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var v map[string]float64
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, false
	}
	return v, true
}
