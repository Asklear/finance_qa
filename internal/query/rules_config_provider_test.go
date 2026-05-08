package query

import (
	"os"
	"testing"
	"time"
)

type staticRuleConfigProvider struct {
	cfg   RuleConfig
	calls int
}

func (p *staticRuleConfigProvider) Current() RuleConfig {
	p.calls++
	return p.cfg
}

func TestEngineResolveQueryRoutingUsesInjectedRuleConfigProvider(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	cfg := defaultRuleConfig()
	setIntentKeywordGroup(&cfg, string(IntentARAPQuery), []string{"自定义挂账"})
	cfg.finalize()
	provider := &staticRuleConfigProvider{cfg: cfg}

	engine, err := NewEngine(dbPath, "测试公司", WithRuleConfigProvider(provider))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("2026年3月自定义挂账多少")
	if provider.calls == 0 {
		t.Fatalf("expected injected rule config provider to be called")
	}
	if route.intent != IntentARAPQuery {
		t.Fatalf("intent = %s, want %s", route.intent, IntentARAPQuery)
	}
	if route.spec.Intent != IntentARAPQuery {
		t.Fatalf("spec intent = %s, want %s", route.spec.Intent, IntentARAPQuery)
	}
}

func TestEngineResolveQueryRoutingUsesInjectedContractPriorityKeywords(t *testing.T) {
	dbPath := buildQueryContextResolutionDB(t)
	cfg := defaultRuleConfig()
	cfg.ContractPriorityKeywordLexicon = []string{"特殊履约"}
	cfg.finalize()
	provider := &staticRuleConfigProvider{cfg: cfg}

	engine, err := NewEngine(dbPath, "测试公司", WithRuleConfigProvider(provider))
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	route := engine.resolveQueryRouting("飞未云科2026年特殊履约合同多少？")
	if provider.calls == 0 {
		t.Fatalf("expected injected rule config provider to be called")
	}
	if route.spec.QueryFamily != QueryFamilyContractDimension {
		t.Fatalf("query family = %s, want %s", route.spec.QueryFamily, QueryFamilyContractDimension)
	}
}

type fakeRuleConfigSource struct {
	path    string
	env     map[string]string
	environ []string
	files   map[string]string
	stats   map[string]os.FileInfo
	reads   int
}

func (s *fakeRuleConfigSource) RulesPath() string {
	return s.path
}

func (s *fakeRuleConfigSource) Environ() []string {
	return append([]string{}, s.environ...)
}

func (s *fakeRuleConfigSource) Getenv(key string) string {
	return s.env[key]
}

func (s *fakeRuleConfigSource) Stat(path string) (os.FileInfo, error) {
	info, ok := s.stats[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return info, nil
}

func (s *fakeRuleConfigSource) ReadFile(path string) ([]byte, error) {
	content, ok := s.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	s.reads++
	return []byte(content), nil
}

type fakeFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i fakeFileInfo) Name() string       { return i.name }
func (i fakeFileInfo) Size() int64        { return i.size }
func (i fakeFileInfo) Mode() os.FileMode  { return 0o600 }
func (i fakeFileInfo) ModTime() time.Time { return i.modTime }
func (i fakeFileInfo) IsDir() bool        { return false }
func (i fakeFileInfo) Sys() any           { return nil }

func TestCachingRuleConfigProviderUsesSourceAndReloadsWhenCacheKeyChanges(t *testing.T) {
	source := &fakeRuleConfigSource{
		path: "rules.json",
		env:  map[string]string{},
		files: map[string]string{
			"rules.json": `{
  "schema_version": 2,
  "router": {
    "intents": {
      "arap": {"keywords": ["文件挂账"]}
    }
  }
}`,
		},
		stats: map[string]os.FileInfo{
			"rules.json": fakeFileInfo{name: "rules.json", size: 100, modTime: time.Unix(100, 0)},
		},
	}
	provider := newCachingRuleConfigProvider(source)

	cfgA := provider.Current()
	if got := cfgA.IntentARAPKeywords; len(got) != 1 || got[0] != "文件挂账" {
		t.Fatalf("cfgA arap keywords = %v, want file config", got)
	}
	cfgCached := provider.Current()
	if got := cfgCached.IntentARAPKeywords; len(got) != 1 || got[0] != "文件挂账" {
		t.Fatalf("cached arap keywords = %v, want file config", got)
	}
	if source.reads != 1 {
		t.Fatalf("source reads = %d, want cached read count 1", source.reads)
	}

	source.env["FINANCEQA_INTENT_ARAP_KEYWORDS"] = "环境挂账"
	source.environ = []string{"FINANCEQA_INTENT_ARAP_KEYWORDS=环境挂账"}
	cfgB := provider.Current()
	if got := cfgB.IntentARAPKeywords; len(got) != 1 || got[0] != "环境挂账" {
		t.Fatalf("cfgB arap keywords = %v, want env override after cache key change", got)
	}
	if source.reads != 2 {
		t.Fatalf("source reads = %d, want reload after cache key change", source.reads)
	}
}
