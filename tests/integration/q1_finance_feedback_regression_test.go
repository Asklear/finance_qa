package integration

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"financeqa/internal/query"
)

type q1FinanceFeedbackCase struct {
	ID             string             `json:"id"`
	Query          string             `json:"query"`
	MustContain    []string           `json:"must_contain"`
	MustNotContain []string           `json:"must_not_contain"`
	ExpectFloats   map[string]float64 `json:"expect_floats"`
	ExpectStrings  map[string]string  `json:"expect_strings"`
	ExpectBools    map[string]bool    `json:"expect_bools"`
}

func TestQ1FinanceFeedbackRegression(t *testing.T) {
	root, _ := requireLiveDBConfig(t)
	engine := requireLiveDBEngine(t, "南京优集数据科技有限公司")

	fixturePath := filepath.Join(root, "tests", "testdata", "q1_finance_feedback_cases.json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}

	var cases []q1FinanceFeedbackCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse fixture %s: %v", fixturePath, err)
	}
	if len(cases) == 0 {
		t.Fatalf("fixture %s is empty", fixturePath)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.ID, func(t *testing.T) {
			start := time.Now()
			res := engine.Query(tc.Query)
			elapsed := time.Since(start)
			t.Logf("query=%q elapsed=%s success=%v sql=%d logs=%d", tc.Query, elapsed.Round(time.Millisecond), res.Success, len(res.ExecutedSQL), len(res.CalculationLogs))

			if !res.Success {
				t.Fatalf("query failed: %s", res.Message)
			}
			if len(res.ExecutedSQL) == 0 || len(res.CalculationLogs) == 0 {
				t.Fatalf("missing trace bundle: sql=%d logs=%d", len(res.ExecutedSQL), len(res.CalculationLogs))
			}

			for _, want := range tc.MustContain {
				if !strings.Contains(res.Message, want) {
					t.Fatalf("message missing %q\nmessage=%s", want, res.Message)
				}
			}
			for _, reject := range tc.MustNotContain {
				if strings.Contains(res.Message, reject) {
					t.Fatalf("message unexpectedly contains %q\nmessage=%s", reject, res.Message)
				}
			}

			rootMap := mustNormalizeResult(t, res)
			for path, want := range tc.ExpectFloats {
				got := mustFloatPath(t, rootMap, path)
				if math.Abs(got-want) > 0.01 {
					t.Fatalf("path %s = %.2f, want %.2f\nresult=%s", path, got, want, marshalResult(t, rootMap))
				}
			}
			for path, want := range tc.ExpectStrings {
				got := mustStringPath(t, rootMap, path)
				if got != want {
					t.Fatalf("path %s = %q, want %q\nresult=%s", path, got, want, marshalResult(t, rootMap))
				}
			}
			for path, want := range tc.ExpectBools {
				got := mustBoolPath(t, rootMap, path)
				if got != want {
					t.Fatalf("path %s = %v, want %v\nresult=%s", path, got, want, marshalResult(t, rootMap))
				}
			}
		})
	}
}

func mustNormalizeResult(t *testing.T, res query.Result) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return root
}

func marshalResult(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal indent: %v", err)
	}
	return string(raw)
}

func mustFloatPath(t *testing.T, root map[string]any, path string) float64 {
	t.Helper()
	v, err := readJSONPath(root, path)
	if err != nil {
		t.Fatalf("read path %s: %v", path, err)
	}
	switch num := v.(type) {
	case float64:
		return num
	case string:
		parsed, err := strconv.ParseFloat(num, 64)
		if err != nil {
			t.Fatalf("path %s string %q is not float: %v", path, num, err)
		}
		return parsed
	default:
		t.Fatalf("path %s expected float, got %T (%v)", path, v, v)
		return 0
	}
}

func mustStringPath(t *testing.T, root map[string]any, path string) string {
	t.Helper()
	v, err := readJSONPath(root, path)
	if err != nil {
		t.Fatalf("read path %s: %v", path, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("path %s expected string, got %T (%v)", path, v, v)
	}
	return s
}

func mustBoolPath(t *testing.T, root map[string]any, path string) bool {
	t.Helper()
	v, err := readJSONPath(root, path)
	if err != nil {
		t.Fatalf("read path %s: %v", path, err)
	}
	b, ok := v.(bool)
	if !ok {
		t.Fatalf("path %s expected bool, got %T (%v)", path, v, v)
	}
	return b
}

var jsonPathSegment = regexp.MustCompile(`^([^.\[\]]+)(?:\[(\d+)\])?$`)

func readJSONPath(root map[string]any, path string) (any, error) {
	current := any(root)
	for _, segment := range strings.Split(path, ".") {
		match := jsonPathSegment.FindStringSubmatch(segment)
		if match == nil {
			return nil, fmt.Errorf("invalid path segment %q", segment)
		}
		key := match[1]
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("segment %q wants map, got %T", segment, current)
		}
		next, ok := m[key]
		if !ok {
			return nil, fmt.Errorf("missing key %q", key)
		}
		current = next
		if match[2] == "" {
			continue
		}
		idx, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, fmt.Errorf("parse index in %q: %w", segment, err)
		}
		items, ok := current.([]any)
		if !ok {
			return nil, fmt.Errorf("segment %q wants slice, got %T", segment, current)
		}
		if idx < 0 || idx >= len(items) {
			return nil, fmt.Errorf("index %d out of range for %q", idx, segment)
		}
		current = items[idx]
	}
	return current, nil
}
