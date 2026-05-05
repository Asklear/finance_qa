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

	mu             sync.Mutex
	token          string
	tokenExpiry    time.Time
	appToken       string
	appTokenExpiry time.Time
	authMode       string
	userTokenFile  string

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

func WithUserTokenFile(path string) Option {
	return func(c *HTTPClient) {
		c.authMode = "user"
		c.userTokenFile = strings.TrimSpace(path)
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
	client := NewHTTPClient(appID, appSecret)
	if strings.EqualFold(strings.TrimSpace(os.Getenv("FEISHU_AUTH_MODE")), "user") {
		tokenFile := strings.TrimSpace(os.Getenv("FEISHU_USER_TOKEN_FILE"))
		if tokenFile == "" {
			return nil, errors.New("FEISHU_USER_TOKEN_FILE is required when FEISHU_AUTH_MODE=user")
		}
		WithUserTokenFile(tokenFile)(client)
	}
	return client, nil
}

func NewHTTPClient(appID, appSecret string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		appID:              strings.TrimSpace(appID),
		appSecret:          strings.TrimSpace(appSecret),
		baseURL:            defaultBaseURL,
		client:             http.DefaultClient,
		authMode:           "tenant",
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

	var batchResp batchMetadataResponse
	body := map[string]any{
		"request_docs": []map[string]string{{
			"doc_token": fileToken,
			"doc_type":  "file",
		}},
		"with_url": true,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/open-apis/drive/v1/metas/batch_query", body, &batchResp); err != nil {
		return DriveFile{}, err
	}
	if err := checkCode(batchResp.Code, batchResp.Msg); err != nil {
		return DriveFile{}, err
	}
	if len(batchResp.Data.Metas) > 0 {
		return batchResp.Data.Metas[0].File(), nil
	}
	if len(batchResp.Data.FailedList) > 0 {
		first := batchResp.Data.FailedList[0]
		return DriveFile{}, APIError{Code: first.Code, Msg: fmt.Sprintf("metadata query failed for %s", first.Token)}
	}
	return DriveFile{}, errors.New("feishu metadata response was empty")
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
	token := ""
	if !isNoAuthPath(path) {
		var err error
		token, err = c.accessToken(ctx)
		if err != nil {
			return err
		}
	}
	return c.doJSONWithBearer(ctx, method, path, body, target, token)
}

func (c *HTTPClient) doJSONWithBearer(ctx context.Context, method, path string, body any, target any, token string) error {
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
	if strings.TrimSpace(token) != "" {
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

	token, err := c.accessToken(ctx)
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

func (c *HTTPClient) accessToken(ctx context.Context) (string, error) {
	if strings.EqualFold(c.authMode, "user") {
		return c.userAccessToken(ctx)
	}
	return c.tenantAccessToken(ctx)
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
	if err := c.doJSONWithBearer(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}, &resp, ""); err != nil {
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

func (c *HTTPClient) appAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.appToken != "" && time.Now().Before(c.appTokenExpiry) {
		token := c.appToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	var resp appTokenResponse
	if err := c.doJSONWithBearer(ctx, http.MethodPost, "/open-apis/auth/v3/app_access_token/internal", map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	}, &resp, ""); err != nil {
		return "", err
	}
	if err := checkCode(resp.Code, resp.Msg); err != nil {
		return "", err
	}
	token := strings.TrimSpace(resp.AppAccessToken)
	if token == "" {
		return "", errors.New("feishu app token response was empty")
	}
	expiresIn := time.Duration(resp.Expire) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Hour
	}

	c.mu.Lock()
	c.appToken = token
	c.appTokenExpiry = time.Now().Add(expiresIn - time.Minute)
	c.mu.Unlock()
	return token, nil
}

func (c *HTTPClient) userAccessToken(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.userTokenFile) == "" {
		return "", errors.New("feishu user token file is required")
	}
	token, err := loadUserToken(c.userTokenFile)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token.AccessToken) != "" && time.Now().Before(token.ExpiresAt.Add(-time.Minute)) {
		return strings.TrimSpace(token.AccessToken), nil
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return "", errors.New("feishu user token expired and refresh_token is empty")
	}
	refreshed, err := c.RefreshUserToken(ctx, token.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := SaveUserToken(c.userTokenFile, refreshed); err != nil {
		return "", err
	}
	return strings.TrimSpace(refreshed.AccessToken), nil
}

func (c *HTTPClient) OAuthURL(redirectURI, state, scope string) string {
	values := url.Values{}
	values.Set("app_id", c.appID)
	values.Set("redirect_uri", strings.TrimSpace(redirectURI))
	if strings.TrimSpace(state) != "" {
		values.Set("state", strings.TrimSpace(state))
	}
	if strings.TrimSpace(scope) != "" {
		values.Set("scope", strings.TrimSpace(scope))
	}
	return c.baseURL + "/open-apis/authen/v1/index?" + values.Encode()
}

func (c *HTTPClient) ExchangeCode(ctx context.Context, code string) (UserToken, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return UserToken{}, errors.New("authorization code is required")
	}
	appToken, err := c.appAccessToken(ctx)
	if err != nil {
		return UserToken{}, err
	}
	var resp userTokenResponse
	if err := c.doJSONWithBearer(ctx, http.MethodPost, "/open-apis/authen/v1/access_token", map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	}, &resp, appToken); err != nil {
		return UserToken{}, err
	}
	if err := checkCode(resp.Code, resp.Msg); err != nil {
		return UserToken{}, err
	}
	return resp.Token(time.Now())
}

func (c *HTTPClient) RefreshUserToken(ctx context.Context, refreshToken string) (UserToken, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return UserToken{}, errors.New("refresh token is required")
	}
	appToken, err := c.appAccessToken(ctx)
	if err != nil {
		return UserToken{}, err
	}
	var resp userTokenResponse
	if err := c.doJSONWithBearer(ctx, http.MethodPost, "/open-apis/authen/v1/refresh_access_token", map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}, &resp, appToken); err != nil {
		return UserToken{}, err
	}
	if err := checkCode(resp.Code, resp.Msg); err != nil {
		return UserToken{}, err
	}
	return resp.Token(time.Now())
}

func isNoAuthPath(path string) bool {
	return path == "/open-apis/auth/v3/tenant_access_token/internal" ||
		path == "/open-apis/auth/v3/app_access_token/internal"
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

type appTokenResponse struct {
	Code           int    `json:"code"`
	Msg            string `json:"msg"`
	AppAccessToken string `json:"app_access_token"`
	Expire         int    `json:"expire"`
}

type UserToken struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
	Scope            string    `json:"scope,omitempty"`
	TokenType        string    `json:"token_type,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type userTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
		Scope            string `json:"scope"`
		TokenType        string `json:"token_type"`
	} `json:"data"`
}

func (r userTokenResponse) Token(now time.Time) (UserToken, error) {
	accessToken := strings.TrimSpace(r.Data.AccessToken)
	if accessToken == "" {
		return UserToken{}, errors.New("feishu user token response was empty")
	}
	expiresIn := time.Duration(r.Data.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Hour
	}
	token := UserToken{
		AccessToken:  accessToken,
		RefreshToken: strings.TrimSpace(r.Data.RefreshToken),
		ExpiresAt:    now.Add(expiresIn),
		Scope:        strings.TrimSpace(r.Data.Scope),
		TokenType:    strings.TrimSpace(r.Data.TokenType),
		UpdatedAt:    now,
	}
	if r.Data.RefreshExpiresIn > 0 {
		token.RefreshExpiresAt = now.Add(time.Duration(r.Data.RefreshExpiresIn) * time.Second)
	}
	return token, nil
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

type batchMetadataResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Metas      []batchMetadataItem `json:"metas"`
		FailedList []struct {
			Token string `json:"token"`
			Code  int    `json:"code"`
		} `json:"failed_list"`
	} `json:"data"`
}

type batchMetadataItem struct {
	DocToken         string `json:"doc_token"`
	DocType          string `json:"doc_type"`
	Title            string `json:"title"`
	LatestModifyTime string `json:"latest_modify_time"`
	URL              string `json:"url"`
}

func (m *batchMetadataResponse) File() (DriveFile, error) {
	if len(m.Data.Metas) == 0 {
		return DriveFile{}, errors.New("feishu metadata response was empty")
	}
	return m.Data.Metas[0].File(), nil
}

func (m *batchMetadataResponse) failedError() error {
	if len(m.Data.FailedList) == 0 {
		return nil
	}
	first := m.Data.FailedList[0]
	return APIError{Code: first.Code, Msg: fmt.Sprintf("metadata query failed for %s", first.Token)}
}

func (m batchMetadataItem) File() DriveFile {
	return DriveFile{
		Token:        strings.TrimSpace(m.DocToken),
		Name:         strings.TrimSpace(m.Title),
		Type:         strings.TrimSpace(m.DocType),
		ModifiedTime: strings.TrimSpace(m.LatestModifyTime),
	}
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
