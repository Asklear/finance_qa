package config_test

import (
	"path/filepath"
	"testing"

	"financeqa/internal/config"
)

func TestUserConfigManagerDefaultsWhenFileMissing(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "user_preferences.yaml")
	mgr, err := config.NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("new user config manager: %v", err)
	}

	cfg := mgr.GetAllConfig()
	if cfg.UserID != "default" {
		t.Fatalf("unexpected default user id: %q", cfg.UserID)
	}
	if cfg.Version != 1 {
		t.Fatalf("unexpected default version: %d", cfg.Version)
	}
	if cfg.ReconciliationRules.ToleranceDays != 1 || cfg.ReconciliationRules.ToleranceAmount != 0.01 {
		t.Fatalf("unexpected default reconciliation rules: %+v", cfg.ReconciliationRules)
	}
}

func TestUserConfigManagerPersistsMetricsAndAliases(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "user_preferences.yaml")
	mgr, err := config.NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("new user config manager: %v", err)
	}

	if ok := mgr.SetMetric("gross_profit", []string{"营业收入"}, []string{"营业外收入"}, "毛利"); !ok {
		t.Fatal("set metric should succeed")
	}
	mgr.SetAccountAlias("工资", "应付职工薪酬")

	loaded, err := config.NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("reload user config manager: %v", err)
	}

	metric := loaded.GetMetric("gross_profit")
	if metric == nil {
		t.Fatal("expected metric to be persisted")
	}
	if metric.Description != "毛利" {
		t.Fatalf("unexpected metric description: %q", metric.Description)
	}
	if got := loaded.ResolveAccountName("工资"); got != "应付职工薪酬" {
		t.Fatalf("expected alias resolution, got %q", got)
	}
}
