package query

import "strings"

var defaultRuleConfigProviderInstance RuleConfigProvider = newCachingRuleConfigProvider(osRuleConfigSource{})

func getRuleConfig() RuleConfig {
	return defaultRuleConfigProviderInstance.Current()
}

// CurrentRuleConfig 返回当前生效规则（默认值 + 文件覆盖 + 环境变量覆盖）。
func CurrentRuleConfig() RuleConfig {
	return getRuleConfig()
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
