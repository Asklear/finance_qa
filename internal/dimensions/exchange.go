package dimensions

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExportFormat is the serialization format for external exchanges.
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// ImportOptions controls import behavior.
type ImportOptions struct {
	ValidateOnly   bool
	SkipExisting   bool
	UpdateExisting bool
	Company        string
}

// ImportPreview summarizes validation-only checks.
type ImportPreview struct {
	Valid        bool          `json:"valid"`
	TotalCount   int           `json:"totalCount"`
	ValidCount   int           `json:"validCount"`
	InvalidCount int           `json:"invalidCount"`
	Errors       []ImportError `json:"errors"`
	Warnings     []string      `json:"warnings"`
	Summary      string        `json:"summary"`
}

// DetailedImportReport tracks import operation outcomes.
type DetailedImportReport struct {
	Success      bool          `json:"success"`
	Operation    string        `json:"operation"`
	TotalCount   int           `json:"totalCount"`
	SuccessCount int           `json:"successCount"`
	FailedCount  int           `json:"failedCount"`
	SkippedCount int           `json:"skippedCount"`
	UpdatedCount int           `json:"updatedCount"`
	CreatedCount int           `json:"createdCount"`
	Errors       []ImportError `json:"errors"`
	Warnings     []string      `json:"warnings"`
	Duration     time.Duration `json:"duration"`
	Message      string        `json:"message"`
}

// DataExchange provides import/export utilities on top of the manager.
type DataExchange struct {
	manager *Manager
}

func NewDataExchange(manager *Manager) *DataExchange {
	return &DataExchange{manager: manager}
}

func (x *DataExchange) ExportFullPackage(ctx context.Context) (ExportDataPackage, error) {
	return x.manager.BuildExportPackage(ctx)
}

func (x *DataExchange) ImportDimensions(ctx context.Context, dims []DimensionExport, opts ImportOptions) DetailedImportReport {
	started := time.Now()
	report := DetailedImportReport{Operation: "import dimensions"}
	report.TotalCount = len(dims)

	for i, dim := range dims {
		code := strings.TrimSpace(dim.Code)
		name := strings.TrimSpace(dim.Name)
		if code == "" || name == "" || !dim.Type.Valid() {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: dim.Code, Name: dim.Name, Reason: "invalid dimension payload"})
			continue
		}

		existing, err := x.manager.GetDimensionByCode(ctx, code)
		if err == nil {
			if opts.SkipExisting {
				report.SkippedCount++
				report.SuccessCount++
				continue
			}
			if !opts.UpdateExisting {
				report.FailedCount++
				report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: "dimension already exists"})
				continue
			}
			if !opts.ValidateOnly {
				t := dim.Type
				_, err = x.manager.UpdateDimension(ctx, existing.ID, DimensionPatch{
					Name:           &name,
					Description:    dim.Description,
					Type:           &t,
					IsActive:       &dim.IsActive,
					IsHierarchical: &dim.IsHierarchical,
				})
				if err != nil {
					report.FailedCount++
					report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
					continue
				}
			}
			report.UpdatedCount++
			report.SuccessCount++
			continue
		}
		if err != ErrNotFound {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
			continue
		}

		if !opts.ValidateOnly {
			_, err = x.manager.CreateDimension(ctx, CreateDimensionInput{
				Code:           code,
				Name:           name,
				Type:           dim.Type,
				Description:    dim.Description,
				IsHierarchical: dim.IsHierarchical,
			})
			if err != nil {
				report.FailedCount++
				report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
				continue
			}
		}
		report.CreatedCount++
		report.SuccessCount++
	}

	report.Duration = time.Since(started)
	report.Success = report.FailedCount == 0
	report.Message = fmt.Sprintf("dimensions imported: created=%d updated=%d failed=%d", report.CreatedCount, report.UpdatedCount, report.FailedCount)
	return report
}

func (x *DataExchange) ImportMembers(ctx context.Context, dimensionCode string, members []MemberExport, opts ImportOptions) DetailedImportReport {
	started := time.Now()
	report := DetailedImportReport{Operation: "import members"}
	report.TotalCount = len(members)

	dim, err := x.manager.GetDimensionByCode(ctx, dimensionCode)
	if err != nil {
		report.Success = false
		report.FailedCount = len(members)
		report.Duration = time.Since(started)
		report.Message = err.Error()
		return report
	}

	createdIDs := make(map[string]int64, len(members))
	for i, member := range members {
		code := strings.TrimSpace(member.Code)
		name := strings.TrimSpace(member.Name)
		if code == "" || name == "" {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: member.Code, Name: member.Name, Reason: "invalid member payload"})
			continue
		}

		existing, err := x.manager.repo.GetMemberByCode(ctx, dim.ID, code)
		if err == nil {
			createdIDs[code] = existing.ID
			if opts.SkipExisting {
				report.SkippedCount++
				report.SuccessCount++
				continue
			}
			if opts.UpdateExisting {
				if !opts.ValidateOnly {
					_, err := x.manager.UpdateMember(ctx, existing.ID, MemberPatch{Name: &name})
					if err != nil {
						report.FailedCount++
						report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
						continue
					}
				}
				report.UpdatedCount++
				report.SuccessCount++
				continue
			}
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: "member already exists"})
			continue
		}
		if err != ErrNotFound {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
			continue
		}
		if !opts.ValidateOnly {
			created, err := x.manager.AddMember(ctx, AddMemberInput{DimensionID: dim.ID, Code: code, Name: name, SortOrder: member.SortOrder})
			if err != nil {
				report.FailedCount++
				report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: code, Name: name, Reason: err.Error()})
				continue
			}
			createdIDs[code] = created.ID
		}
		report.CreatedCount++
		report.SuccessCount++
	}

	if !opts.ValidateOnly {
		for _, member := range members {
			if member.ParentCode == nil {
				continue
			}
			childID, childOK := createdIDs[member.Code]
			parentID, parentOK := createdIDs[*member.ParentCode]
			if !childOK || !parentOK {
				continue
			}
			_, err := x.manager.UpdateMember(ctx, childID, MemberPatch{ParentIDSet: true, ParentID: &parentID})
			if err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("failed to set parent for %s: %v", member.Code, err))
			}
		}
	}

	report.Duration = time.Since(started)
	report.Success = report.FailedCount == 0
	report.Message = fmt.Sprintf("members imported: created=%d updated=%d failed=%d", report.CreatedCount, report.UpdatedCount, report.FailedCount)
	return report
}

func (x *DataExchange) ImportMappingRules(ctx context.Context, rules []MappingRuleExport, opts ImportOptions) DetailedImportReport {
	started := time.Now()
	report := DetailedImportReport{Operation: "import mapping rules"}
	report.TotalCount = len(rules)

	for i, rule := range rules {
		company := strings.TrimSpace(rule.Company)
		if opts.Company != "" {
			company = opts.Company
		}
		if company == "" || strings.TrimSpace(rule.RuleName) == "" {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: rule.RuleName, Name: rule.DimensionCode, Reason: "invalid mapping rule payload"})
			continue
		}

		existing, err := x.manager.repo.GetMappingRuleByName(ctx, company, rule.RuleName)
		if err == nil {
			if opts.SkipExisting {
				report.SkippedCount++
				report.SuccessCount++
				continue
			}
			if !opts.UpdateExisting {
				report.FailedCount++
				report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: rule.RuleName, Name: rule.DimensionCode, Reason: "mapping rule already exists"})
				continue
			}
			if !opts.ValidateOnly {
				_, err := x.manager.UpdateMappingRule(ctx, existing.ID, MappingRulePatch{
					Priority:            &rule.Priority,
					AccountCodePattern:  rule.AccountCodePattern,
					AccountNamePattern:  rule.AccountNamePattern,
					SummaryPattern:      rule.SummaryPattern,
					CounterpartyPattern: rule.CounterpartyPattern,
					DimensionCode:       &rule.DimensionCode,
					MemberCode:          &rule.MemberCode,
					AllocationRatio:     &rule.AllocationRatio,
					ValidFrom:           rule.ValidFrom,
					ValidTo:             rule.ValidTo,
					IsActive:            &rule.IsActive,
				})
				if err != nil {
					report.FailedCount++
					report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: rule.RuleName, Name: rule.DimensionCode, Reason: err.Error()})
					continue
				}
			}
			report.UpdatedCount++
			report.SuccessCount++
			continue
		}
		if err != ErrNotFound {
			report.FailedCount++
			report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: rule.RuleName, Name: rule.DimensionCode, Reason: err.Error()})
			continue
		}

		if !opts.ValidateOnly {
			isActive := rule.IsActive
			_, err = x.manager.CreateMappingRule(ctx, CreateMappingRuleInput{
				Company:             company,
				RuleName:            rule.RuleName,
				Priority:            rule.Priority,
				AccountCodePattern:  rule.AccountCodePattern,
				AccountNamePattern:  rule.AccountNamePattern,
				SummaryPattern:      rule.SummaryPattern,
				CounterpartyPattern: rule.CounterpartyPattern,
				DimensionCode:       rule.DimensionCode,
				MemberCode:          rule.MemberCode,
				AllocationRatio:     rule.AllocationRatio,
				ValidFrom:           rule.ValidFrom,
				ValidTo:             rule.ValidTo,
				IsActive:            &isActive,
			})
			if err != nil {
				report.FailedCount++
				report.Errors = append(report.Errors, ImportError{Row: i + 1, Code: rule.RuleName, Name: rule.DimensionCode, Reason: err.Error()})
				continue
			}
		}

		report.CreatedCount++
		report.SuccessCount++
	}

	report.Duration = time.Since(started)
	report.Success = report.FailedCount == 0
	report.Message = fmt.Sprintf("mapping rules imported: created=%d updated=%d failed=%d", report.CreatedCount, report.UpdatedCount, report.FailedCount)
	return report
}

func (x *DataExchange) PreviewDimensionsImport(ctx context.Context, dims []DimensionExport) ImportPreview {
	report := x.ImportDimensions(ctx, dims, ImportOptions{ValidateOnly: true})
	return previewFromReport(report)
}

func (x *DataExchange) PreviewMembersImport(ctx context.Context, dimensionCode string, members []MemberExport) ImportPreview {
	report := x.ImportMembers(ctx, dimensionCode, members, ImportOptions{ValidateOnly: true})
	return previewFromReport(report)
}

func previewFromReport(report DetailedImportReport) ImportPreview {
	return ImportPreview{
		Valid:        report.FailedCount == 0,
		TotalCount:   report.TotalCount,
		ValidCount:   report.SuccessCount,
		InvalidCount: report.FailedCount,
		Errors:       report.Errors,
		Warnings:     report.Warnings,
		Summary:      fmt.Sprintf("total=%d valid=%d invalid=%d", report.TotalCount, report.SuccessCount, report.FailedCount),
	}
}
