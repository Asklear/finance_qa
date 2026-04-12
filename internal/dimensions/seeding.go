package dimensions

import (
	"context"
	"fmt"
)

type standardAccount struct {
	Code   string
	Name   string
	Level  int
	Parent string
}

func getStandardCASChart() []standardAccount {
	return []standardAccount{
		{"1002", "银行存款", 1, ""},
		{"1601", "固定资产", 1, ""},
		{"160101", "固定资产-房屋及建筑物", 2, "1601"},
		{"1122", "应收账款", 1, ""},
		{"2202", "应付账款", 1, ""},
		{"2211", "应付职工薪酬", 1, ""},
		{"2221", "应交税费", 1, ""},
		{"222101", "应交增值税-销项", 2, "2221"},
		{"222102", "应交增值税-进项", 2, "2221"},
		{"6001", "营业收入", 1, ""},
		{"6051", "其他业务收入", 1, ""},
		{"6401", "营业成本", 1, ""},
		{"6403", "税金及附加", 1, ""},
		{"6601", "销售费用", 1, ""},
		{"6602", "管理费用", 1, ""},
		{"6603", "财务费用", 1, ""},
		{"6301", "营业外收入", 1, ""},
		{"6711", "营业外支出", 1, ""},
		{"6801", "所得税费用", 1, ""},
	}
}

// InitializeStandardRules populates the CAS dimension and creates default mapping rules for a company.
func (m *Manager) InitializeStandardRules(ctx context.Context, company string) error {
	// ... (dimensions setup remains same)
	standardDims := []struct {
		Code string
		Name string
	}{
		{"CAS", "中国会计准则标准科目"},
		{"PROJECT", "项目基地档案"},
		{"CUSTOMER", "客户档案"},
		{"SUPPLIER", "供应商档案"},
	}

	for _, d := range standardDims {
		_, err := m.GetDimensionByCode(ctx, d.Code)
		if err != nil {
			_, err = m.CreateDimension(ctx, CreateDimensionInput{
				Code: d.Code,
				Name: d.Name,
				Type: DimensionTypeCustom,
			})
			if err != nil {
				return fmt.Errorf("create %s dimension: %w", d.Code, err)
			}
		}
	}

	// 2. Populate "CAS" dimension members from standard chart (Hierarchical)
	dim, _ := m.GetDimensionByCode(ctx, "CAS")
	accounts := getStandardCASChart()
	codeToID := make(map[string]int64)

	// Pass 1: Level 1
	for _, acc := range accounts {
		if acc.Level == 1 {
			member, err := m.AddMember(ctx, AddMemberInput{
				DimensionID: dim.ID,
				Code:        acc.Code,
				Name:        acc.Name,
			})
			if err == nil || err == ErrAlreadyExists {
				if err == ErrAlreadyExists {
					if m, err := m.repo.GetMemberByCode(ctx, dim.ID, acc.Code); err == nil {
						codeToID[acc.Code] = m.ID
					}
				} else {
					codeToID[acc.Code] = member.ID
				}
			}
		}
	}

	// Pass 2: Level 2
	for _, acc := range accounts {
		if acc.Level == 2 {
			var parentID *int64
			if acc.Parent != "" {
				if id, ok := codeToID[acc.Parent]; ok {
					parentID = &id
				}
			}
			member, err := m.AddMember(ctx, AddMemberInput{
				DimensionID: dim.ID,
				Code:        acc.Code,
				Name:        acc.Name,
				ParentID:    parentID,
			})
			if err == nil || err == ErrAlreadyExists {
				if err == ErrAlreadyExists {
					if m, err := m.repo.GetMemberByCode(ctx, dim.ID, acc.Code); err == nil {
						codeToID[acc.Code] = m.ID
					}
				} else {
					codeToID[acc.Code] = member.ID
				}
			}
		}
	}

	// 3. Create default mapping rules for the company
	isActive := true
	for _, acc := range accounts {
		// Create rules for both Level 1 (4-digits) and Level 2 (6-digits)
		if len(acc.Code) >= 4 {
			codePattern := acc.Code + "%"
			// Priority: Level 2 rules get higher priority so they match first
			priority := 100
			if acc.Level == 2 {
				priority = 110
			}

			_, _ = m.CreateMappingRule(ctx, CreateMappingRuleInput{
				Company:            company,
				RuleName:           fmt.Sprintf("Auto-map CAS %s", acc.Code),
				Priority:           priority,
				AccountCodePattern: &codePattern,
				DimensionCode:      "CAS",
				MemberCode:         acc.Code,
				IsActive:           &isActive,
			})
		}
	}

	return nil
}
