package query

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (e *Engine) collectContractDimensionSummary(question, entity string, anchor time.Time) (contractDimensionSummary, error) {
	from, to := extractContractQuestionPeriods(question, anchor)
	return e.collectContractDimensionSummaryForPeriod(question, entity, from, to)
}

func (e *Engine) collectContractDimensionSummaryForPeriod(question, entity, from, to string) (contractDimensionSummary, error) {
	cfg := e.currentRuleConfig()
	subPeriod, hasSubPeriod := extractReceiptSubPeriod(question, from, to)
	askedTopic := inferContractAskedTopic(question)

	if resolved := e.resolveContractSubject(question, entity); resolved != "" {
		entity = resolved
	}
	if strings.TrimSpace(entity) == "" {
		return contractDimensionSummary{}, errors.New("contract entity not found")
	}

	contracts := e.queryMatchingContractsForQuestion(question, entity)
	if len(contracts) == 0 {
		return contractDimensionSummary{}, errors.New("contract not found")
	}

	role := "unknown"
	if askedTopic != "content" {
		role = e.detectContractRole(entity, from, to)
		if role == "unknown" {
			return contractDimensionSummary{}, errors.New("contract role not found")
		}
	}

	summary := newContractDimensionSummary(entity, role, from, to, askedTopic, contracts, cfg)
	if askedTopic == "content" {
		summary.Role = "contract_content"
		summary.Data["role"] = "contract_content"
		summary.Data["source_tables"] = contractSourceTablesForRoleWithConfig("contract_content", cfg)
		applyContractPerspectiveAliases(summary.Data)
		return summary, nil
	}

	like := "%" + entity + "%"
	var err error
	switch role {
	case "customer_contract":
		summary, err = e.collectCustomerContractSummary(summary, like, hasSubPeriod, subPeriod)
	case "supplier_contract":
		summary, err = e.collectSupplierContractSummary(summary, like)
	case "mixed_contract":
		summary, err = e.collectMixedContractSummary(summary, like)
	default:
		err = errors.New("unsupported contract role")
	}
	if err != nil {
		return contractDimensionSummary{}, err
	}

	applyContractPerspectiveAliases(summary.Data)
	return summary, nil
}

func newContractDimensionSummary(entity, role, from, to, askedTopic string, contracts []contractDimensionRow, cfg RuleConfig) contractDimensionSummary {
	periodLabel := displayPeriod(from, to)
	contractList := make([]map[string]any, 0, len(contracts))
	for _, contract := range contracts {
		contractList = append(contractList, map[string]any{
			"contract_id":      contract.ContractID,
			"customer_name":    contract.CustomerName,
			"contract_content": contract.ContractContent,
		})
	}

	return contractDimensionSummary{
		Entity:     entity,
		Role:       role,
		Period:     periodLabel,
		PeriodFrom: from,
		PeriodTo:   to,
		Contracts:  contractList,
		Data: map[string]any{
			"entity":         entity,
			"role":           role,
			"period":         periodLabel,
			"period_from":    from,
			"period_to":      to,
			"contract_count": len(contracts),
			"contracts":      contractList,
			"asked_topic":    askedTopic,
			"source_tables":  contractSourceTablesForRoleWithConfig(role, cfg),
		},
		ExecutedSQL: []string{
			"contract_lookup: SELECT contract_id, customer_name, contract_content FROM fin_contracts WHERE customer_name LIKE ? OR contract_content LIKE ? ORDER BY contract_id",
		},
		CalculationLog: []string{
			fmt.Sprintf("[合同维度] entity=%s period=%s matched_contracts=%d", entity, periodLabel, len(contracts)),
		},
	}
}
