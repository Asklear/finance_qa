package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://open.feishu.cn"

type HTTPClient struct {
	appID     string
	appSecret string
	baseURL   string
	client    *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time

	exportPollInterval time.Duration
	exportMaxAttempts  int
}

type Option func(*HTTPClient)

func WithBaseURL(baseURL string) Option {
	return func(c *HTTPClient) {
		c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *HTTPClient) {
		if client != nil {
			c.client = client
		}
	}
}

func WithExportPollInterval(interval time.Duration) Option {
	return func(c *HTTPClient) {
		if interval > 0 {
			c.exportPollInterval = interval
		}
	}
}

func WithExportMaxAttempts(maxAttempts int) Option {
	return func(c *HTTPClient) {
		if maxAttempts > 0 {
			c.exportMaxAttempts = maxAttempts
		}
	}
}

func NewHTTPClientFromEnv() (*HTTPClient, error) {
	appID := strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	if appID == "" || appSecret == "" {
		return nil, errors.New("FEISHU_APP_ID and FEISHU_APP_SECRET are required")
	}
	return NewHTTPClient(appID, appSecret), nil
}

func NewHTTPClient(appID, appSecret string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		appID:              strings.TrimSpace(appID),
		appSecret:          strings.TrimSpace(appSecret),
		baseURL:            defaultBaseURL,
		client:             http.DefaultClient,
		exportPollInterval: 2 * time.Second,
		exportMaxAttempts:  60,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *HTTPClient) ListFolderFiles(ctx context.Context, folderToken string) ([]DriveFile, error) {
	folderToken = strings.TrimSpace(folderToken)
	if folderToken == "" {
		return nil, errors.New("folder token is required")
	}

	var out []DriveFile
	pageToken := ""
	for {
		values := url.Values{}
		values.Set("folder_token", folderToken)
		values.Set("page_size", "200")
		if pageToken != "" {
			values.Set("page_token", pageToken)
		}

		var resp listFilesResponse
		if err := c.doJSON(ctx, http.MethodGet, "/open-apis/drive/v1/files?"+values.Encode(), nil, &resp); err != nil {
			return nil, err
		}
		if err := checkCode(resp.Code, resp.Msg); err != nil {
			return nil, err
		}
		out = append(out, resp.Data.Files...)
		if !resp.Data.HasMore {
			break
		}
		pageToken = strings.TrimSpace(resp.Data.PageToken)
		if pageToken == "" {
			break
		}
	}
	return out, nil
}

func (c *HTTPClient) GetFileMetadata(ctx context.Context, fileToken string) (DriveFile, error) {
	fileToken = strings.TrimSpace(fileToken)
	if fileToken == "" {
		return DriveFile{}, errors.New("file token is required")
	}

	var resp fileMetadataResponse
	if err := c.doJSON(ctx, http.MethodGet, "/open-apis/drive/v1/files/"+url.PathEscape(fileToken), nil, &resp); err != nil {
		return DriveFile{}, err
	}
	if err := checkCode(resp.Code, resp.Msg); err != nil {
		return DriveFile{}, err
	}
	return resp.File()
}

func (c *HTTPClient) DownloadFile(ctx context.Context, fileToken, destPath string) error {
	fileToken = strings.TrimSpace(fileToken)
	if fileToken == "" {
		return errors.New("file token is required")
	}
	return c.download(ctx, "/open-apis/drive/v1/files/"+url.PathEscape(fileToken)+"/download", destPath)
}

func (c *HTTPClient) ExportToXLSX(ctx context.Context, fileToken, destPath string) error {
	fileToken = strings.TrimSpace(fileToken)
	if fileToken == "" {
		return errors.New("file token is required")
	}

	var createResp exportTaskCreateResponse
	body := map[string]string{
		"token":          fileToken,
		"type":           "sheet",
		"file_extension": "xlsx",
	}
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/drive/v1/export_tasks", body, &createResp); err != nil {
		return err
	}
	if err := checkCode(createResp.Code, createResp.Msg); err != nil {
		return err
	}
	ticket := strings.TrimSpace(createResp.Data.Ticket)
	if ticket == "" {
		return errors.New("feishu export task did not return a ticket")
	}

	var fileTokenOut string
	for attempt := 0; attempt < c.exportMaxAttempts; attempt++ {
		var getResp exportTaskGetResponse
		if err := c.doJSON(ctx, http.MethodGet, "/open-apis/drive/v1/export_tasks/"+url.PathEscape(ticket), nil, &getResp); err != nil {
			return err
		}
		if err := checkCode(getResp.Code, getResp.Msg); err != nil {
			return err
		}
		if strings.TrimSpace(getResp.Data.Result.JobErrorMsg) != "" {
			return fmt.Errorf("feishu export failed: %s", strings.TrimSpace(getResp.Data.Result.JobErrorMsg))
		}
		if getResp.Data.Result.JobStatus == 0 && strings.TrimSpace(getResp.Data.Result.FileToken) != "" {
			fileTokenOut = strings.TrimSpace(getResp.Data.Result.FileToken)
			break
		}
		timer := time.NewTimer(c.exportPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if fileTokenOut == "" {
		return errors.New("feishu export task timed out")
	}

	return c.download(ctx, "/open-apis/drive/v1/export_tasks/file/"+url.PathEscape(fileTokenOut)+"/download", destPath)
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if path != "/open-apis/auth/v3/tenant_access_token/internal" {
		token, err := c.tenantAccessToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("feishu http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *HTTPClient) download(ctx context.Context, path, destPath string) error {
	destPath = strings.TrimSpace(destPath)
	if destPath == "" {
		return errors.New("destination path is required")
	}

	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("feishu download http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	return os.Rename(tmpPath, destPath)
}

func (c *HTTPClient) tenantAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	var resp tenantTokenResponse
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}, &resp); err != nil {
		return "", err
	}
	if err := checkCode(resp.Code, resp.Msg); err != nil {
		return "", err
	}
	token := strings.TrimSpace(resp.TenantAccessToken)
	if token == "" {
		return "", errors.New("feishu tenant token response was empty")
	}
	expiresIn := time.Duration(resp.Expire) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Hour
	}

	c.mu.Lock()
	c.token = token
	c.tokenExpiry = time.Now().Add(expiresIn - time.Minute)
	c.mu.Unlock()
	return token, nil
}

func checkCode(code int, msg string) error {
	if code == 0 {
		return nil
	}
	return APIError{Code: code, Msg: msg}
}

type tenantTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

type listFilesResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Files     []DriveFile `json:"files"`
		HasMore   bool        `json:"has_more"`
		PageToken string      `json:"page_token"`
	} `json:"data"`
}

type fileMetadataResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (r fileMetadataResponse) File() (DriveFile, error) {
	var nested struct {
		File DriveFile `json:"file"`
	}
	if err := json.Unmarshal(r.Data, &nested); err == nil && strings.TrimSpace(nested.File.Token) != "" {
		return nested.File, nil
	}
	var direct DriveFile
	if err := json.Unmarshal(r.Data, &direct); err != nil {
		return DriveFile{}, err
	}
	return direct, nil
}

type exportTaskCreateResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Ticket string `json:"ticket"`
	} `json:"data"`
}

type exportTaskGetResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Result struct {
			JobStatus   int    `json:"job_status"`
			FileToken   string `json:"file_token"`
			JobErrorMsg string `json:"job_error_msg"`
		} `json:"result"`
	} `json:"data"`
}

func (f *DriveFile) UnmarshalJSON(data []byte) error {
	type alias DriveFile
	var raw struct {
		alias
		Size any `json:"size"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = DriveFile(raw.alias)
	f.Size = parseInt64(raw.Size)
	return nil
}

func parseInt64(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 0
	case float64:
		return int64(v)
	case int64:
		return v
	case json.Number:
		out, _ := v.Int64()
		return out
	case string:
		out, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return out
	default:
		return 0
	}
}
