package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultGeminiModel          = "gemini-3.1-flash-lite-preview"
	defaultGeminiBaseURL        = "https://generativelanguage.googleapis.com/v1beta"
	defaultGeminiTimeout        = 240 * time.Second
	defaultInputCostPerMillion  = 0.25
	defaultOutputCostPerMillion = 1.50
)

type GeminiConfig struct {
	APIKey                  string
	Model                   string
	BaseURL                 string
	ProxyURL                string
	Timeout                 time.Duration
	InputCostPerMillionUSD  float64
	OutputCostPerMillionUSD float64
	MaxFileBytes            int64
	Prompt                  string
	HTTPClient              *http.Client
	FileResolver            PDFResolver
}

type GeminiClient struct {
	config GeminiConfig
	client *http.Client
}

type PDFResolver interface {
	ResolvePDF(ctx context.Context, storageKey string) (localPath string, cleanup func(), err error)
}

func NewGeminiClient(config GeminiConfig) *GeminiClient {
	if strings.TrimSpace(config.Model) == "" {
		config.Model = DefaultGeminiModel
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultGeminiBaseURL
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultGeminiTimeout
	}
	if config.InputCostPerMillionUSD <= 0 {
		config.InputCostPerMillionUSD = defaultInputCostPerMillion
	}
	if config.OutputCostPerMillionUSD <= 0 {
		config.OutputCostPerMillionUSD = defaultOutputCostPerMillion
	}
	if strings.TrimSpace(config.Prompt) == "" {
		config.Prompt = GeminiExtractionPrompt()
	}
	client := config.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if proxy := strings.TrimSpace(config.ProxyURL); proxy != "" {
			if proxyURL, err := url.Parse(proxy); err == nil {
				transport.Proxy = http.ProxyURL(proxyURL)
			}
		}
		client = &http.Client{Transport: transport, Timeout: config.Timeout}
	}
	return &GeminiClient{config: config, client: client}
}

func (c *GeminiClient) ExtractPDF(ctx context.Context, filePath string) (Result, RunMetadata, error) {
	start := time.Now()
	if strings.TrimSpace(c.config.APIKey) == "" {
		return Result{}, RunMetadata{}, errors.New("GEMINI_API_KEY is required")
	}
	localPath := filePath
	if c.config.FileResolver != nil {
		resolved, cleanup, err := c.config.FileResolver.ResolvePDF(ctx, filePath)
		if err != nil {
			return Result{}, RunMetadata{}, fmt.Errorf("resolve pdf: %w", err)
		}
		if cleanup != nil {
			defer cleanup()
		}
		localPath = resolved
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("read pdf: %w", err)
	}
	if c.config.MaxFileBytes > 0 && int64(len(data)) > c.config.MaxFileBytes {
		return Result{}, RunMetadata{}, fmt.Errorf("pdf too large: %d bytes exceeds %d", len(data), c.config.MaxFileBytes)
	}

	payload := geminiRequest{
		Contents: []geminiContent{{
			Role: "user",
			Parts: []geminiPart{
				{Text: c.config.Prompt},
				{InlineData: &geminiInlineData{
					MimeType: "application/pdf",
					Data:     base64.StdEncoding.EncodeToString(data),
				}},
			},
		}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:      0,
			ResponseMimeType: "application/json",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("marshal gemini request: %w", err)
	}
	endpoint := strings.TrimRight(c.config.BaseURL, "/") + "/models/" + url.PathEscape(c.config.Model) + ":generateContent?key=" + url.QueryEscape(c.config.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("create gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("call gemini: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("read gemini response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, RunMetadata{}, fmt.Errorf("gemini http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("decode gemini response: %w", err)
	}
	text := apiResp.firstText()
	if strings.TrimSpace(text) == "" {
		return Result{}, RunMetadata{}, errors.New("gemini response did not include text")
	}
	result, err := decodeGeminiResultText(text)
	if err != nil {
		return Result{}, RunMetadata{}, fmt.Errorf("decode gemini json text: %w", err)
	}

	usage := Usage{
		InputTokens:  apiResp.UsageMetadata.PromptTokenCount,
		OutputTokens: apiResp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:  apiResp.UsageMetadata.TotalTokenCount,
	}
	meta := RunMetadata{
		Model:          c.config.Model,
		ElapsedSeconds: time.Since(start).Seconds(),
		Usage:          usage,
		ProcessedAt:    time.Now().UTC(),
		EstimatedCostUSD: (float64(usage.InputTokens)/1_000_000)*c.config.InputCostPerMillionUSD +
			(float64(usage.OutputTokens)/1_000_000)*c.config.OutputCostPerMillionUSD,
	}
	return result, meta, nil
}

func decodeGeminiResultText(text string) (Result, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		if newline := strings.IndexByte(trimmed, '\n'); newline >= 0 {
			trimmed = strings.TrimSpace(trimmed[newline+1:])
		}
		if strings.HasSuffix(trimmed, "```") {
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
		}
	}
	if firstObject := firstJSONObject(trimmed); firstObject != "" {
		trimmed = firstObject
	}
	var result Result
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	if err := decoder.Decode(&result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func firstJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for idx := start; idx < len(text); idx++ {
		ch := text[idx]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : idx+1]
			}
			if depth < 0 {
				return ""
			}
		}
	}
	return ""
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature"`
	ResponseMimeType string  `json:"responseMimeType"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (r geminiResponse) firstText() string {
	if len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return ""
	}
	return r.Candidates[0].Content.Parts[0].Text
}
