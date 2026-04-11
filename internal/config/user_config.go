package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"financeqa/internal/support"
	"financeqa/internal/types"
)

func defaultUserConfig() types.UserConfig {
	return types.UserConfig{
		UserID:         "default",
		Version:        1,
		Metrics:        map[string]types.ConfigMetric{},
		AccountAliases: map[string]string{},
		ReconciliationRules: types.ReconciliationRules{
			ToleranceDays:   1,
			ToleranceAmount: 0.01,
		},
	}
}

type UserConfigManager struct {
	path string

	mu     sync.RWMutex
	config types.UserConfig
}

var (
	defaultUserConfigManager *UserConfigManager
	defaultUserConfigErr     error
	defaultUserConfigOnce    sync.Once
)

func GetConfigManager() (*UserConfigManager, error) {
	defaultUserConfigOnce.Do(func() {
		defaultUserConfigManager, defaultUserConfigErr = NewUserConfigManager("")
	})
	return defaultUserConfigManager, defaultUserConfigErr
}

func NewUserConfigManager(path string) (*UserConfigManager, error) {
	if path == "" {
		path = support.DefaultUserConfigPath("")
	}
	if err := support.EnsureParentDir(path); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	mgr := &UserConfigManager{path: path, config: defaultUserConfig()}
	if err := mgr.load(); err != nil {
		return nil, err
	}

	return mgr, nil
}

func (m *UserConfigManager) load() error {
	content, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			m.config = defaultUserConfig()
			return nil
		}
		return fmt.Errorf("read config file: %w", err)
	}

	cfg := defaultUserConfig()
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return fmt.Errorf("parse config yaml: %w", err)
	}

	cfg = normalizeConfig(cfg)
	m.config = cfg
	return nil
}

func normalizeConfig(cfg types.UserConfig) types.UserConfig {
	if cfg.UserID == "" {
		cfg.UserID = "default"
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Metrics == nil {
		cfg.Metrics = map[string]types.ConfigMetric{}
	}
	if cfg.AccountAliases == nil {
		cfg.AccountAliases = map[string]string{}
	}
	if cfg.ReconciliationRules.ToleranceDays == 0 {
		cfg.ReconciliationRules.ToleranceDays = 1
	}
	if cfg.ReconciliationRules.ToleranceAmount == 0 {
		cfg.ReconciliationRules.ToleranceAmount = 0.01
	}
	return cfg
}

func (m *UserConfigManager) saveLocked() error {
	content, err := yaml.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("marshal config yaml: %w", err)
	}
	if err := os.WriteFile(m.path, content, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func (m *UserConfigManager) GetMetric(name string) *types.ConfigMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metric, ok := m.config.Metrics[name]
	if !ok {
		return nil
	}
	copyMetric := metric
	return &copyMetric
}

func (m *UserConfigManager) SetMetric(name string, accounts, exclude []string, description string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.Metrics == nil {
		m.config.Metrics = map[string]types.ConfigMetric{}
	}
	if description == "" {
		description = "custom metric: " + name
	}

	m.config.Metrics[name] = types.ConfigMetric{
		Description: description,
		Accounts:    append([]string(nil), accounts...),
		Exclude:     append([]string(nil), exclude...),
	}

	return m.saveLocked() == nil
}

func (m *UserConfigManager) DeleteMetric(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.config.Metrics[name]; !ok {
		return false
	}
	delete(m.config.Metrics, name)
	return m.saveLocked() == nil
}

func (m *UserConfigManager) ListMetrics() map[string]types.ConfigMetric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]types.ConfigMetric, len(m.config.Metrics))
	for name, metric := range m.config.Metrics {
		metrics[name] = metric
	}
	return metrics
}

func (m *UserConfigManager) GetAccountAliases() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	aliases := make(map[string]string, len(m.config.AccountAliases))
	for k, v := range m.config.AccountAliases {
		aliases[k] = v
	}
	return aliases
}

func (m *UserConfigManager) SetAccountAlias(alias, actualName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config.AccountAliases == nil {
		m.config.AccountAliases = map[string]string{}
	}
	m.config.AccountAliases[alias] = actualName
	_ = m.saveLocked()
}

func (m *UserConfigManager) ResolveAccountName(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if actual, ok := m.config.AccountAliases[name]; ok {
		return actual
	}
	return name
}

func (m *UserConfigManager) GetReconciliationRules() types.ReconciliationRules {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.ReconciliationRules
}

func (m *UserConfigManager) SetReconciliationRules(toleranceDays *int, toleranceAmount *float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if toleranceDays != nil {
		m.config.ReconciliationRules.ToleranceDays = *toleranceDays
	}
	if toleranceAmount != nil {
		m.config.ReconciliationRules.ToleranceAmount = *toleranceAmount
	}
	_ = m.saveLocked()
}

func (m *UserConfigManager) GetAllConfig() types.UserConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.config
	cfg.Metrics = make(map[string]types.ConfigMetric, len(m.config.Metrics))
	for k, v := range m.config.Metrics {
		cfg.Metrics[k] = v
	}
	cfg.AccountAliases = make(map[string]string, len(m.config.AccountAliases))
	for k, v := range m.config.AccountAliases {
		cfg.AccountAliases[k] = v
	}
	return cfg
}
