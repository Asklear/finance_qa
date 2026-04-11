package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func main() {
	questions := []string{
		"三月收入",
		"合作伙伴A客户今年销售额多少",
		"这个月整体支出多少",
		"人力成本多少",
		"阿里云计算有限公司供应商支出多少",
		"合作伙伴A 3月数据出来了吗",
		"内部研发项目1月收入多少",
		"内部研发项目1月成本多少",
		"1月销项税额是多少",
		"1月进项税额是多少",
		"1月总成本是多少",
		"1月应收账款多少",
		"1月应付账款多少",
		"研发项目的应收",
		"公司财务健康度怎么样",
	}

	fmt.Println("============ 2026年Q1 业务回归测试对照表 ============")
	fmt.Println("数据源：模拟财务科技有限公司 (序时帐+银行流)")
	fmt.Println("| 编号 | 测试问题 | 系统回答核心内容 | 耗时 |")
	fmt.Println("| --- | --- | --- | --- |")

	for i, q := range questions {
		start := time.Now()
		cmd := exec.Command("go", "run", "cmd/financeqa/main.go", "query", "--company", "模拟财务科技有限公司", q)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Run()
		duration := time.Since(start)

		outStr := strings.TrimSpace(out.String())
		lines := strings.SplitN(outStr, "\n", 2)
		
		answer := "<解析失败或无数据>"
		if len(lines) > 0 {
			msg := lines[0]
			var data map[string]any
			
			if len(lines) > 1 {
				jsonStr := strings.TrimSpace(lines[1])
				// remove trailing blank lines and standard stdout noise
				if strings.HasPrefix(jsonStr, "{") {
					json.Unmarshal([]byte(jsonStr), &data)
				}
			}
			
			dataStr := ""
			for k, v := range data {
				if vf, ok := v.(float64); ok {
					dataStr += fmt.Sprintf("<br>• **%s**: %.2f", k, vf)
				} else if m, ok := v.(map[string]any); ok {
					dataStr += fmt.Sprintf("<br>• **[%s]**", k)
					for subk, subv := range m {
						if subvf, ok := subv.(float64); ok {
							dataStr += fmt.Sprintf("<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ %s: %.2f", subk, subvf)
						} else {
							dataStr += fmt.Sprintf("<br>&nbsp;&nbsp;&nbsp;&nbsp;◦ %s: %v", subk, subv)
						}
					}
				} else {
					dataStr += fmt.Sprintf("<br>• **%s**: %v", k, v)
				}
			}
			
			answer = fmt.Sprintf("✅ **%s**%s", msg, dataStr)
			if strings.Contains(msg, "没有查到") || strings.Contains(msg, "未识别") || strings.Contains(msg, "失败") {
				answer = fmt.Sprintf("⚠️ %s", msg)
			}
		}
		
		durStr := fmt.Sprintf("%d ms", duration.Milliseconds())
		fmt.Printf("| %d | %s | %s | %s |\n", i+1, q, answer, durStr)
	}
}
