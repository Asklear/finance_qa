package testutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func NormalizeJSON(t testing.TB, v any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return root
}

func MarshalJSON(t testing.TB, v any) string {
	t.Helper()

	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal indent: %v", err)
	}
	return string(raw)
}

func MustFloatPath(t testing.TB, root map[string]any, path string) float64 {
	t.Helper()

	v, err := ReadJSONPath(root, path)
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

func MustStringPath(t testing.TB, root map[string]any, path string) string {
	t.Helper()

	v, err := ReadJSONPath(root, path)
	if err != nil {
		t.Fatalf("read path %s: %v", path, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("path %s expected string, got %T (%v)", path, v, v)
	}
	return s
}

func MustBoolPath(t testing.TB, root map[string]any, path string) bool {
	t.Helper()

	v, err := ReadJSONPath(root, path)
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

func ReadJSONPath(root map[string]any, path string) (any, error) {
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
