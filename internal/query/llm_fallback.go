package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type llmQueryPlan struct {
	Intent     string `json:"intent"`
	EntityType string `json:"entity_type"`
	EntityName string `json:"entity_name"`
	Metric     string `json:"metric"`
	Account    string `json:"account"`
	PeriodFrom string `json:"period_from"`
	PeriodTo   string `json:"period_to"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (e *Engine) queryLLMFallback(question, from, to string) Result {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return Result{Success: false, Message: "llm fallback disabled: OPENAI_API_KEY not set"}
	}

	model := strings.TrimSpace(os.Getenv("FINANCEQA_LLM_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}

	plan, err := generateLLMPlan(apiKey, model, question, e.Company, from, to)
	if err != nil {
		return Result{Success: false, Message: fmt.Sprintf("llm fallback failed: %v", err)}
	}

	switch strings.ToLower(strings.TrimSpace(plan.Intent)) {
	case "monthly_summary":
		metricQuestion := question
		if plan.Metric != "" {
			metricQuestion = fmt.Sprintf("%s%s是多少", plan.PeriodTo, plan.Metric)
		}
		return e.queryMonthlySummary(metricQuestion, nonEmpty(plan.PeriodFrom, from), nonEmpty(plan.PeriodTo, to))
	case "tax":
		return e.queryTax(nonEmpty(plan.PeriodFrom, from), nonEmpty(plan.PeriodTo, to))
	case "ar_ap":
		accountQ := question
		if plan.Metric != "" {
			accountQ = plan.Metric
		}
		return e.queryARAP(accountQ, nonEmpty(plan.PeriodTo, to))
	case "analysis":
		return e.queryAnalysis(nonEmpty(plan.PeriodTo, to))
	case "entity_count":
		return e.queryEntityCountFallback(question, nonEmpty(plan.PeriodFrom, from), nonEmpty(plan.PeriodTo, to))
	case "counterparty_amount":
		rewritten := question
		if plan.EntityName != "" {
			metric := nonEmpty(plan.Metric, "收入")
			rewritten = fmt.Sprintf("%s客户%s多少", plan.EntityName, metric)
		}
		return e.queryCounterpartyAmountFallback(rewritten, nonEmpty(plan.PeriodFrom, from), nonEmpty(plan.PeriodTo, to))
	case "account_balance":
		rewritten := question
		if plan.Account != "" {
			rewritten = fmt.Sprintf("%s余额是多少", plan.Account)
		}
		return e.queryPrecise(rewritten, nonEmpty(plan.PeriodTo, to))
	default:
		return Result{Success: false, Message: "llm fallback returned unsupported intent"}
	}
}

func generateLLMPlan(apiKey, model, question, company, from, to string) (llmQueryPlan, error) {
	systemPrompt := `You are a finance QA intent parser.
Return ONLY a JSON object with these fields:
intent, entity_type, entity_name, metric, account, period_from, period_to.
Allowed intent: monthly_summary, tax, ar_ap, analysis, entity_count, counterparty_amount, account_balance.
Use empty string when unknown.
No markdown.`

	userPrompt := fmt.Sprintf(`company=%s
question=%s
default_period_from=%s
default_period_to=%s`, company, question, from, to)

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llmQueryPlan{}, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llmQueryPlan{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return llmQueryPlan{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmQueryPlan{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llmQueryPlan{}, fmt.Errorf("openai status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return llmQueryPlan{}, err
	}
	if len(chatResp.Choices) == 0 {
		return llmQueryPlan{}, fmt.Errorf("empty choices from llm")
	}
	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if content == "" {
		return llmQueryPlan{}, fmt.Errorf("empty llm content")
	}

	plan := llmQueryPlan{}
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return llmQueryPlan{}, err
	}
	if strings.TrimSpace(plan.PeriodFrom) == "" {
		plan.PeriodFrom = from
	}
	if strings.TrimSpace(plan.PeriodTo) == "" {
		plan.PeriodTo = to
	}
	return plan, nil
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
