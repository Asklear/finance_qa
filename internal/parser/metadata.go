package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	bankDateRangePattern = regexp.MustCompile(`(\d{8})-(\d{8})`)
	yearPattern          = regexp.MustCompile(`(\d{4})`)
	periodPattern        = regexp.MustCompile(`(\d{4})[年\.](\d{1,2})(?:-(\d{1,2}))?`)
	companyPattern       = regexp.MustCompile(`^(?:\d{8}_\d{6}_)?(.+?)(\d{4})`)
)

func DetectReportType(path string) string {
	filename := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(filename, "交易查询"), strings.Contains(filename, "银行"), strings.Contains(filename, "流水"):
		return "bank_statement"
	case strings.Contains(filename, "序时账"), strings.Contains(filename, "凭证"):
		return "journal"
	case strings.Contains(filename, "余额表"):
		return "balance_detail"
	case strings.Contains(filename, "资产负债表"):
		return "balance_sheet"
	case strings.Contains(filename, "利润表"), strings.Contains(filename, "损益表"):
		return "income_statement"
	default:
		return "unknown"
	}
}

func ExtractMetadata(path string) (FileMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileMetadata{}, err
	}

	filename := filepath.Base(path)
	reportType := DetectReportType(filename)
	company := "Unknown"
	periodStart := ""
	periodEnd := ""

	switch {
	case strings.Contains(filename, "交易查询"):
		if m := bankDateRangePattern.FindStringSubmatch(filename); len(m) == 3 {
			periodStart = formatYYYYMM(m[1][:6])
			periodEnd = formatYYYYMM(m[2][:6])
		}
		// Extract company from bank statement: 交易查询，南京优集数据科技有限公司，...
		parts := strings.Split(filename, "，")
		if len(parts) >= 2 {
			company = strings.TrimSpace(parts[1])
		} else {
			// Try comma (English)
			parts = strings.Split(filename, ",")
			if len(parts) >= 2 {
				company = strings.TrimSpace(parts[1])
			}
		}
	default:
		if m := companyPattern.FindStringSubmatch(filename); len(m) >= 2 {
			extracted := strings.TrimSpace(strings.TrimRight(m[1], "._-"))
			// Avoid single-character or numeric-only company names
			if len([]rune(extracted)) >= 2 {
				company = extracted
			}
		}
		if m := periodPattern.FindStringSubmatch(filename); len(m) >= 3 {
			periodStart = m[1] + "-" + pad2(m[2])
			if len(m) >= 4 && m[3] != "" {
				periodEnd = m[1] + "-" + pad2(m[3])
			} else {
				periodEnd = periodStart
			}
		}
	}

	return FileMetadata{
		Filename:     filename,
		FilePath:     path,
		FileSize:     info.Size(),
		ModifiedTime: info.ModTime().Format(time.RFC3339),
		ReportType:   reportType,
		Company:      sanitizeCompanyName(company),
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
	}, nil
}

func sanitizeCompanyName(name string) string {
	if name == "" || name == "Unknown" {
		return "DefaultCompany"
	}
	return name
}

func formatYYYYMM(v string) string {
	if len(v) != 6 {
		return v
	}
	return v[:4] + "-" + v[4:]
}

func pad2(v string) string {
	if len(v) >= 2 {
		return v
	}
	return "0" + v
}
