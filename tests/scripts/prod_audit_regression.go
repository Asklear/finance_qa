package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type AuditQuestion struct {
	ID       int
	Question string
}

func main() {
	company := "南京优集数据科技有限公司"
	questions := []AuditQuestion{
		{1, "2026年1月和2月的总收入是多少？"},
		{2, "飞未云科(深圳)技术有限公司今年总计销售额"},
		{3, "2026年2月整体支出多少"},
		{4, "1月人力成本（应付职工薪酬）"},
		{5, "供应商有多少个？"},
		{6, "南京林悦智能科技有限公司数据出来了吗"},
		{7, "梁梦瑶报销了多少钱"},
		{8, "飞未云科(深圳)技术有限公司支付的成本是多少"},
		{9, "2026年2月销项税额是多少"},
		{10, "2026年2月进项税额是多少"},
		{11, "2026年2月总成本"},
		{12, "资产负债表：2026年2月货币资金余额"},
		{13, "当前的应收账款汇总"},
		{14, "南京市中闻（南京）律师事务所的付款记录"},
		{15, "公司经营状况深度评估"},
	}

	fmt.Println("# 🚀 南京优集生产数据：全量回归审计报告 (Version 2.0)")
	fmt.Printf("> 生成时间: %s | 锚定账期: 2026-02\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("| ID | 审计提问 | 状态 | 核心回答内容 | 逻辑校验结果 | 耗时 |")
	fmt.Println("|:---|:--- |:--- |:--- |:--- |:---|")

	for _, aq := range questions {
		start := time.Now()
		cmd := exec.Command("go", "run", "cmd/financeqa/main.go", "query", "--company", company, aq.Question)
		var out bytes.Buffer
		cmd.Stdout = &out
		_ = cmd.Run()
		duration := time.Since(start)

		resultStr := strings.TrimSpace(out.String())
		
		status := "❌"
		content := "解析失败"
		logicLog := "N/A"
		
		var data struct {
			Success         bool     `json:"success"`
			Message         string   `json:"message"`
			CalculationLogs []string `json:"calculation_logs"`
		}

		if err := json.Unmarshal([]byte(resultStr), &data); err == nil {
			if data.Success {
				status = "✅"
			} else if strings.Contains(data.Message, "未查到") || strings.Contains(data.Message, "没有查到") {
				status = "⚠️"
			}
			content = data.Message
			if len(data.CalculationLogs) > 0 {
				logicLog = data.CalculationLogs[0]
			}
		} else {
			content = resultStr
		}

		fmt.Printf("| %d | %s | %s | %s | %s | %dms |\n", 
			aq.ID, aq.Question, status, content, logicLog, duration.Milliseconds())
	}
}
