package query

import (
	"os"
	"sort"
	"strconv"
	"strings"
)

func getRuleConfig() RuleConfig {
	cacheKey := currentRuleConfigCacheKey()
	ruleConfigCacheMu.RLock()
	if cacheKey != "" && cacheKey == ruleConfigCacheKey {
		cfg := ruleConfigCacheData
		ruleConfigCacheMu.RUnlock()
		return cfg
	}
	ruleConfigCacheMu.RUnlock()

	cfg := buildRuleConfig(currentRuleConfigPath(), os.Getenv)

	ruleConfigCacheMu.Lock()
	ruleConfigCacheKey = cacheKey
	ruleConfigCacheData = cfg
	ruleConfigCacheMu.Unlock()
	return cfg
}

// CurrentRuleConfig 返回当前生效规则（默认值 + 文件覆盖 + 环境变量覆盖）。
func CurrentRuleConfig() RuleConfig {
	return getRuleConfig()
}

func currentRuleConfigPath() string {
	return strings.TrimSpace(os.Getenv("FINANCEQA_RULES_PATH"))
}

func currentRuleConfigCacheKey() string {
	filtered := prefixedRuleConfigEnvEntries(os.Environ(), "FINANCEQA_")
	sort.Strings(filtered)

	var b strings.Builder
	for _, entry := range filtered {
		b.WriteString(entry)
		b.WriteByte('\n')
	}

	path := currentRuleConfigPath()
	if path != "" {
		if stat, err := os.Stat(path); err == nil {
			b.WriteString("rules_path_stat=")
			b.WriteString(path)
			b.WriteByte('|')
			b.WriteString(strconv.FormatInt(stat.Size(), 10))
			b.WriteByte('|')
			b.WriteString(strconv.FormatInt(stat.ModTime().UnixNano(), 10))
		} else {
			b.WriteString("rules_path_missing=")
			b.WriteString(path)
		}
	}

	return b.String()
}

func prefixedRuleConfigEnvEntries(envs []string, prefix string) []string {
	filtered := make([]string, 0, len(envs))
	for _, entry := range envs {
		if strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
