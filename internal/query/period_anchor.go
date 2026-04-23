package query

import (
	"strings"
	"time"
)

func parseAnchorDateValue(v any) (time.Time, bool) {
	switch raw := v.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return raw, !raw.IsZero()
	case string:
		return parseAnchorDateString(raw)
	case []byte:
		return parseAnchorDateString(string(raw))
	default:
		return time.Time{}, false
	}
}

func parseAnchorDateString(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	if len(raw) >= len("2006-01-02") {
		if t, err := time.Parse("2006-01-02", raw[:10]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
