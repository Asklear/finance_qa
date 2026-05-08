package query

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// RuleConfigProvider supplies the effective query rule configuration.
type RuleConfigProvider interface {
	// Current returns the effective rule configuration for the current request.
	Current() RuleConfig
}

type ruleConfigSource interface {
	RulesPath() string
	Environ() []string
	Getenv(string) string
	Stat(string) (os.FileInfo, error)
	ReadFile(string) ([]byte, error)
}

type osRuleConfigSource struct{}

func (osRuleConfigSource) RulesPath() string {
	return strings.TrimSpace(os.Getenv("FINANCEQA_RULES_PATH"))
}

func (osRuleConfigSource) Environ() []string {
	return os.Environ()
}

func (osRuleConfigSource) Getenv(key string) string {
	return os.Getenv(key)
}

func (osRuleConfigSource) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (osRuleConfigSource) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

type cachingRuleConfigProvider struct {
	source ruleConfigSource

	mu       sync.RWMutex
	cacheKey string
	cache    RuleConfig
}

func newCachingRuleConfigProvider(source ruleConfigSource) *cachingRuleConfigProvider {
	if source == nil {
		source = osRuleConfigSource{}
	}
	return &cachingRuleConfigProvider{source: source}
}

func (p *cachingRuleConfigProvider) Current() RuleConfig {
	cacheKey := p.currentCacheKey()
	p.mu.RLock()
	if cacheKey != "" && cacheKey == p.cacheKey {
		cfg := p.cache
		p.mu.RUnlock()
		return cfg
	}
	p.mu.RUnlock()

	cfg := buildRuleConfigWithReader(p.source.RulesPath(), p.source.Getenv, p.source.ReadFile)

	p.mu.Lock()
	p.cacheKey = cacheKey
	p.cache = cfg
	p.mu.Unlock()
	return cfg
}

func (p *cachingRuleConfigProvider) currentCacheKey() string {
	filtered := prefixedRuleConfigEnvEntries(p.source.Environ(), "FINANCEQA_")
	sort.Strings(filtered)

	var b strings.Builder
	for _, entry := range filtered {
		b.WriteString(entry)
		b.WriteByte('\n')
	}

	path := strings.TrimSpace(p.source.RulesPath())
	if path != "" {
		if stat, err := p.source.Stat(path); err == nil {
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

func (e *Engine) currentRuleConfig() RuleConfig {
	if e != nil && e.ruleConfigProvider != nil {
		return e.ruleConfigProvider.Current()
	}
	return getRuleConfig()
}
