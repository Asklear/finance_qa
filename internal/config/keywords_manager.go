package config

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"

	"financeqa/internal/support"
	"financeqa/internal/types"
)

// Legacy query keyword manager.
//
// New query-layer keyword and lexicon rules should go to config/rules.json and
// be loaded via internal/query/rules_config.go. This manager remains only for
// backward compatibility with older query_keywords.json consumers.
func defaultKeywordsConfig() types.KeywordsConfig {
	return types.KeywordsConfig{
		Intents: map[string]types.IntentConfig{
			"monthly_summary": {
				Keywords:        []string{"整体", "汇总", "总计", "本月", "这个月", "总收入", "总支出"},
				SpecialKeywords: []string{"利润"},
			},
		},
		TransactionTypes: map[string]types.TransactionTypeConfig{
			"expense": {
				Keywords:    []string{"支出", "成本", "付款", "费用"},
				SQLField:    "debit_amount",
				Description: "expense transactions",
			},
			"income": {
				Keywords:    []string{"收入", "销售", "收款"},
				SQLField:    "credit_amount",
				Description: "income transactions",
			},
			"profit": {
				Keywords:    []string{"利润", "盈利", "收益"},
				Calculation: "income_minus_expense",
				Description: "profit calculation",
			},
		},
		DatabaseFields: map[string]types.DatabaseFieldConfig{},
	}
}

type KeywordsManager struct {
	path string
	cfg  types.KeywordsConfig
	raw  map[string]any
}

var (
	defaultKeywordsManager *KeywordsManager
	defaultKeywordsOnce    sync.Once
)

func GetKeywordsManager() *KeywordsManager {
	defaultKeywordsOnce.Do(func() {
		defaultKeywordsManager = NewKeywordsManager("")
	})
	return defaultKeywordsManager
}

func NewKeywordsManager(path string) *KeywordsManager {
	if path == "" {
		path = support.DefaultKeywordsPath("")
	}

	mgr := &KeywordsManager{path: path}
	mgr.load()
	return mgr
}

func (m *KeywordsManager) load() {
	defaultCfg := defaultKeywordsConfig()
	defaultRaw := mustToRaw(defaultCfg)

	content, err := os.ReadFile(m.path)
	if err != nil {
		m.cfg = defaultCfg
		m.raw = defaultRaw
		return
	}

	cfg := types.KeywordsConfig{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		m.cfg = defaultCfg
		m.raw = defaultRaw
		return
	}
	cfg = normalizeKeywordsConfig(cfg)

	raw := map[string]any{}
	if err := json.Unmarshal(content, &raw); err != nil {
		raw = mustToRaw(cfg)
	}

	m.cfg = cfg
	m.raw = raw
}

func normalizeKeywordsConfig(cfg types.KeywordsConfig) types.KeywordsConfig {
	if cfg.Intents == nil {
		cfg.Intents = map[string]types.IntentConfig{}
	}
	if cfg.TransactionTypes == nil {
		cfg.TransactionTypes = map[string]types.TransactionTypeConfig{}
	}
	if cfg.DatabaseFields == nil {
		cfg.DatabaseFields = map[string]types.DatabaseFieldConfig{}
	}
	return cfg
}

func mustToRaw(cfg types.KeywordsConfig) map[string]any {
	b, err := json.Marshal(cfg)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func (m *KeywordsManager) Get(path string, defaultValue any) any {
	if path == "" {
		return defaultValue
	}

	keys := strings.Split(path, ".")
	var value any = m.raw
	for _, key := range keys {
		obj, ok := value.(map[string]any)
		if !ok {
			return defaultValue
		}
		next, ok := obj[key]
		if !ok {
			return defaultValue
		}
		value = next
	}

	if value == nil {
		return defaultValue
	}
	return value
}

func (m *KeywordsManager) CheckKeywordsInText(text, path string) bool {
	keywords := toStringSlice(m.Get(path, []any{}))
	if len(keywords) == 0 {
		return false
	}

	textLower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(textLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func (m *KeywordsManager) GetCalculationType(text string) string {
	for key, cfg := range m.cfg.TransactionTypes {
		for _, kw := range cfg.Keywords {
			if strings.Contains(strings.ToLower(text), strings.ToLower(kw)) {
				return key
			}
		}
	}
	return ""
}

func (m *KeywordsManager) GetSQLField(calcType string) string {
	cfg, ok := m.cfg.TransactionTypes[calcType]
	if !ok {
		return ""
	}
	return cfg.SQLField
}

func (m *KeywordsManager) GetDatabaseFields(tableName string) (types.DatabaseFieldConfig, bool) {
	fields, ok := m.cfg.DatabaseFields[tableName]
	return fields, ok
}

func (m *KeywordsManager) HasMonthlySummarySpecialKeyword(text string) bool {
	keywords := toStringSlice(m.Get("intents.monthly_summary.special_keywords", []any{}))
	if len(keywords) == 0 {
		return false
	}

	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func (m *KeywordsManager) GetIntentNames() []string {
	names := make([]string, 0, len(m.cfg.Intents))
	for name := range m.cfg.Intents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *KeywordsManager) GetIntentConfig(intentName string) (types.IntentConfig, bool) {
	intent, ok := m.cfg.Intents[intentName]
	return intent, ok
}

func toStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
