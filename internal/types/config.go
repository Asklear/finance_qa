package types

// ConfigMetric maps directly to user metric customizations.
type ConfigMetric struct {
	Description string   `yaml:"description" json:"description"`
	Accounts    []string `yaml:"accounts" json:"accounts"`
	Exclude     []string `yaml:"exclude" json:"exclude"`
}

// ReconciliationRules mirrors tolerance settings from the TypeScript config manager.
type ReconciliationRules struct {
	ToleranceDays   int     `yaml:"toleranceDays" json:"toleranceDays"`
	ToleranceAmount float64 `yaml:"toleranceAmount" json:"toleranceAmount"`
}

// UserConfig is the YAML shape for config/user_preferences.yaml.
type UserConfig struct {
	UserID              string                  `yaml:"userId" json:"userId"`
	Version             int                     `yaml:"version" json:"version"`
	Metrics             map[string]ConfigMetric `yaml:"metrics" json:"metrics"`
	AccountAliases      map[string]string       `yaml:"accountAliases" json:"accountAliases"`
	ReconciliationRules ReconciliationRules     `yaml:"reconciliationRules" json:"reconciliationRules"`
}

// CalculationTypeConfig is used by keyword-driven calculation helpers.
type CalculationTypeConfig struct {
	Keywords    []string `json:"keywords"`
	SQLField    string   `json:"sql_field,omitempty"`
	Calculation string   `json:"calculation,omitempty"`
	Description string   `json:"description,omitempty"`
}

// IntentConfig contains intent-level keyword matching rules.
type IntentConfig struct {
	Keywords          []string                         `json:"keywords,omitempty"`
	SimplePatterns    []string                         `json:"simple_patterns,omitempty"`
	ComplexPatterns   []string                         `json:"complex_patterns,omitempty"`
	PrimaryKeywords   []string                         `json:"primary_keywords,omitempty"`
	SecondaryKeywords []string                         `json:"secondary_keywords,omitempty"`
	SuffixPatterns    []string                         `json:"suffix_patterns,omitempty"`
	Patterns          []string                         `json:"patterns,omitempty"`
	SubIntents        map[string][]string              `json:"sub_intents,omitempty"`
	SpecialKeywords   []string                         `json:"special_keywords,omitempty"`
	CalculationTypes  map[string]CalculationTypeConfig `json:"calculation_types,omitempty"`
	Description       string                           `json:"description,omitempty"`
}

// DatabaseFieldConfig maps source table names to relevant fields.
type DatabaseFieldConfig struct {
	IncomeField           string `json:"income_field,omitempty"`
	ExpenseField          string `json:"expense_field,omitempty"`
	DateField             string `json:"date_field,omitempty"`
	CounterpartyField     string `json:"counterparty_field,omitempty"`
	CompanyField          string `json:"company_field,omitempty"`
	CurrentAmountField    string `json:"current_amount_field,omitempty"`
	CumulativeAmountField string `json:"cumulative_amount_field,omitempty"`
	ItemNameField         string `json:"item_name_field,omitempty"`
	PeriodField           string `json:"period_field,omitempty"`
	OpeningField          string `json:"opening_field,omitempty"`
	ClosingField          string `json:"closing_field,omitempty"`
	AccountNameField      string `json:"account_name_field,omitempty"`
}

// TransactionTypeConfig stores keyword-to-field calculation mapping.
type TransactionTypeConfig struct {
	Keywords    []string `json:"keywords"`
	SQLField    string   `json:"sql_field,omitempty"`
	Calculation string   `json:"calculation,omitempty"`
	Description string   `json:"description,omitempty"`
}

// KeywordsConfig is the top-level JSON payload in query_keywords.json.
type KeywordsConfig struct {
	Intents          map[string]IntentConfig          `json:"intents"`
	DatabaseFields   map[string]DatabaseFieldConfig   `json:"database_fields"`
	TransactionTypes map[string]TransactionTypeConfig `json:"transaction_types"`
}
