package support

import (
	"fmt"
	"strings"
)

func BuildSourceSummary(data map[string]any, message string) string {
	primaryDocs := anyToStringSlice(data["source_documents"])
	supportingDocs := anyToStringSlice(data["supporting_source_documents"])
	if len(primaryDocs) > 0 || len(supportingDocs) > 0 {
		return buildSourceSummaryLine(primaryDocs, supportingDocs)
	}

	for _, key := range []string{"source_summary", "source_note"} {
		if text := strings.TrimSpace(anyToString(data[key])); text != "" {
			return text
		}
	}

	return extractSourceLineFromMessage(message)
}

func buildSourceSummaryLine(primaryDocs, supportingDocs []string) string {
	primaryDocs = dedupeStrings(primaryDocs)
	supportingDocs = dedupeStrings(supportingDocs)
	if len(primaryDocs) == 0 && len(supportingDocs) == 0 {
		return ""
	}
	if len(primaryDocs) == 0 {
		return "来源：" + strings.Join(supportingDocs, "；")
	}
	line := "来源：" + strings.Join(primaryDocs, "；")
	if len(supportingDocs) > 0 {
		line += "；补充参考：" + strings.Join(supportingDocs, "；")
	}
	return line
}

func extractSourceLineFromMessage(message string) string {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "来源：") {
			return line
		}
	}
	return ""
}

func anyToStringSlice(v any) []string {
	switch rows := v.(type) {
	case nil:
		return nil
	case []string:
		return dedupeStrings(rows)
	case []any:
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			text := strings.TrimSpace(anyToString(row))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return dedupeStrings(out)
	default:
		text := strings.TrimSpace(anyToString(v))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func anyToString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	default:
		return strings.TrimSpace(fmt.Sprint(val))
	}
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
