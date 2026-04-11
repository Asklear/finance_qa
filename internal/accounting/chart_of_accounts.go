// Package accounting provides Chinese accounting standard definitions and
// financial calculation utilities.
package accounting

// AccountCategory classifies accounts into standard Chinese accounting categories.
type AccountCategory string

const (
	CategoryAsset     AccountCategory = "asset"     // 资产类
	CategoryLiability AccountCategory = "liability"  // 负债类
	CategoryEquity    AccountCategory = "equity"     // 所有者权益类
	CategoryRevenue   AccountCategory = "revenue"    // 收入类（损益类）
	CategoryExpense   AccountCategory = "expense"    // 费用/成本类（损益类）
)

// StandardAccount represents one entry in the standard chart of accounts.
type StandardAccount struct {
	Code      string          `json:"code"`
	Name      string          `json:"name"`
	Category  AccountCategory `json:"category"`
	Level     int             `json:"level"`
	Parent    string          `json:"parent"`
	Direction string          `json:"direction"` // 正常余额方向: "借" 或 "贷"
}

// IsProfitLoss returns true if this account is a revenue or expense account
// (i.e., it participates in the income statement / P&L).
func (a *StandardAccount) IsProfitLoss() bool {
	return a.Category == CategoryRevenue || a.Category == CategoryExpense
}

// CategoryForCode returns the category for a given account code prefix.
// This uses the Chinese small-enterprise accounting standard numbering.
func CategoryForCode(code string) AccountCategory {
	if len(code) < 1 {
		return CategoryAsset
	}
	switch code[0] {
	case '1':
		return CategoryAsset
	case '2':
		return CategoryLiability
	case '3':
		return CategoryEquity
	case '4': // 4103 本年利润 is special, treat as equity
		return CategoryEquity
	case '5':
		return CategoryExpense // 成本类
	case '6':
		// 6001/6301 = revenue; 6401/6402/6403/6601/6602/6603 = expense
		if len(code) >= 4 {
			prefix := code[:4]
			switch prefix {
			case "6001", "6051", "6301":
				return CategoryRevenue
			}
		}
		return CategoryExpense
	default:
		return CategoryAsset
	}
}

// NormalDirection returns the normal balance direction for a category.
func NormalDirection(cat AccountCategory) string {
	switch cat {
	case CategoryAsset, CategoryExpense:
		return "借"
	default:
		return "贷"
	}
}

// StandardChartOfAccounts returns the standard chart of accounts for Chinese
// small enterprises (小企业会计准则). This is a fixed national standard.
func StandardChartOfAccounts() []StandardAccount {
	return []StandardAccount{
		// ===== 资产类 (1xxx) =====
		{Code: "1001", Name: "库存现金", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1002", Name: "银行存款", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1012", Name: "其他货币资金", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1101", Name: "短期投资", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1121", Name: "应收票据", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1122", Name: "应收账款", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1123", Name: "预付账款", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1131", Name: "应收股利", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1132", Name: "应收利息", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1221", Name: "其他应收款", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1401", Name: "材料采购", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1403", Name: "原材料", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1405", Name: "库存商品", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1601", Name: "固定资产", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1602", Name: "累计折旧", Category: CategoryAsset, Level: 1, Direction: "贷"},
		{Code: "1604", Name: "在建工程", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1701", Name: "无形资产", Category: CategoryAsset, Level: 1, Direction: "借"},
		{Code: "1702", Name: "累计摊销", Category: CategoryAsset, Level: 1, Direction: "贷"},
		{Code: "1801", Name: "长期待摊费用", Category: CategoryAsset, Level: 1, Direction: "借"},

		// ===== 负债类 (2xxx) =====
		{Code: "2001", Name: "短期借款", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2201", Name: "应付票据", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2202", Name: "应付账款", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2203", Name: "预收账款", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2211", Name: "应付职工薪酬", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2221", Name: "应交税费", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2231", Name: "应付利息", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2232", Name: "应付股利", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2241", Name: "其他应付款", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2501", Name: "长期借款", Category: CategoryLiability, Level: 1, Direction: "贷"},
		{Code: "2701", Name: "长期应付款", Category: CategoryLiability, Level: 1, Direction: "贷"},

		// ===== 所有者权益类 (3xxx) =====
		{Code: "3001", Name: "实收资本", Category: CategoryEquity, Level: 1, Direction: "贷"},
		{Code: "3002", Name: "资本公积", Category: CategoryEquity, Level: 1, Direction: "贷"},
		{Code: "3101", Name: "盈余公积", Category: CategoryEquity, Level: 1, Direction: "贷"},
		{Code: "3103", Name: "本年利润", Category: CategoryEquity, Level: 1, Direction: "贷"},
		{Code: "3104", Name: "利润分配", Category: CategoryEquity, Level: 1, Direction: "贷"},

		// 注: 实际使用中 4103 也表示本年利润
		{Code: "4103", Name: "本年利润", Category: CategoryEquity, Level: 1, Direction: "贷"},

		// ===== 收入类 (6001/6051/6301) =====
		{Code: "6001", Name: "主营业务收入", Category: CategoryRevenue, Level: 1, Direction: "贷"},
		{Code: "6051", Name: "其他业务收入", Category: CategoryRevenue, Level: 1, Direction: "贷"},
		{Code: "6301", Name: "营业外收入", Category: CategoryRevenue, Level: 1, Direction: "贷"},

		// ===== 成本/费用类 (5xxx, 6401+) =====
		{Code: "5001", Name: "生产成本", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "5101", Name: "制造费用", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6401", Name: "主营业务成本", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6402", Name: "其他业务成本", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6403", Name: "税金及附加", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6601", Name: "销售费用", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6602", Name: "管理费用", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6603", Name: "财务费用", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6711", Name: "营业外支出", Category: CategoryExpense, Level: 1, Direction: "借"},
		{Code: "6801", Name: "所得税费用", Category: CategoryExpense, Level: 1, Direction: "借"},
	}
}

// AccountLookup provides fast code → StandardAccount lookup.
type AccountLookup map[string]*StandardAccount

// BuildLookup creates a lookup map from the standard chart.
func BuildLookup() AccountLookup {
	chart := StandardChartOfAccounts()
	lookup := make(AccountLookup, len(chart))
	for i := range chart {
		lookup[chart[i].Code] = &chart[i]
	}
	return lookup
}

// FindParentCode returns the nearest standard parent code for a given code.
// For example, "600101" → "6001", "66020101" → "6602".
func (l AccountLookup) FindParentCode(code string) string {
	// Try progressively shorter prefixes
	for length := len(code) - 1; length >= 4; length-- {
		prefix := code[:length]
		if _, ok := l[prefix]; ok {
			return prefix
		}
	}
	// Try first 4 digits
	if len(code) >= 4 {
		return code[:4]
	}
	return code
}
