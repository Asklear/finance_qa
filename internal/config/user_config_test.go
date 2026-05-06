package config

import (
	"path/filepath"
	"testing"
)

func TestUserConfigManagerPersistsUserPreferences(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "nested", "user_preferences.yaml")
	mgr, err := NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("new user config manager: %v", err)
	}

	if ok := mgr.SetMetric("gross_profit", []string{"6001"}, []string{"6301"}, ""); !ok {
		t.Fatal("set metric should succeed")
	}
	mgr.SetAccountAlias("工资", "应付职工薪酬")
	days := 3
	amount := 1.25
	mgr.SetReconciliationRules(&days, &amount)

	reloaded, err := NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("reload user config manager: %v", err)
	}

	metric := reloaded.GetMetric("gross_profit")
	if metric == nil {
		t.Fatal("expected persisted metric")
	}
	if metric.Description != "custom metric: gross_profit" {
		t.Fatalf("metric description = %q", metric.Description)
	}
	if got := reloaded.ResolveAccountName("工资"); got != "应付职工薪酬" {
		t.Fatalf("resolved account = %q, want alias target", got)
	}
	if rules := reloaded.GetReconciliationRules(); rules.ToleranceDays != 3 || rules.ToleranceAmount != 1.25 {
		t.Fatalf("unexpected reconciliation rules: %+v", rules)
	}
	if !reloaded.DeleteMetric("gross_profit") {
		t.Fatal("delete existing metric should return true")
	}
	if reloaded.DeleteMetric("gross_profit") {
		t.Fatal("delete missing metric should return false")
	}
}

func TestUserConfigManagerReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "user_preferences.yaml")
	mgr, err := NewUserConfigManager(configPath)
	if err != nil {
		t.Fatalf("new user config manager: %v", err)
	}
	if ok := mgr.SetMetric("revenue", []string{"6001"}, []string{"6051"}, "收入"); !ok {
		t.Fatal("set metric should succeed")
	}

	metric := mgr.GetMetric("revenue")
	metric.Accounts[0] = "BAD"
	metric.Exclude[0] = "BAD"
	if got := mgr.GetMetric("revenue"); got.Accounts[0] != "6001" || got.Exclude[0] != "6051" {
		t.Fatalf("GetMetric exposed mutable slices: %+v", got)
	}

	listed := mgr.ListMetrics()
	listed["revenue"].Accounts[0] = "BAD"
	if got := mgr.GetMetric("revenue"); got.Accounts[0] != "6001" {
		t.Fatalf("ListMetrics exposed mutable slices: %+v", got)
	}

	all := mgr.GetAllConfig()
	all.Metrics["revenue"].Accounts[0] = "BAD"
	all.AccountAliases["x"] = "y"
	if got := mgr.GetMetric("revenue"); got.Accounts[0] != "6001" {
		t.Fatalf("GetAllConfig exposed mutable metric slices: %+v", got)
	}
	if got := mgr.ResolveAccountName("x"); got != "x" {
		t.Fatalf("GetAllConfig exposed account alias map, got %q", got)
	}
}
