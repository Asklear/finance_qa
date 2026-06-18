package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrOSSNotConfigured = errors.New("oss is not configured")

type OSSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	Endpoint        string
}

type S3Ref struct {
	Bucket string
	Key    string
}

type OSSClient struct {
	config OSSConfig
	client *http.Client
}

func NewOSSClient(config OSSConfig) *OSSClient {
	config.Endpoint = normalizeEndpoint(config.Endpoint)
	return &OSSClient{config: config, client: &http.Client{Timeout: 120 * time.Second}}
}

func NewOSSClientFromEnv() (*OSSClient, error) {
	config := OSSConfig{
		AccessKeyID:     strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")),
		AccessKeySecret: strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_SECRET")),
		Bucket:          strings.TrimSpace(os.Getenv("OSS_BUCKET")),
		Endpoint:        strings.TrimSpace(os.Getenv("OSS_ENDPOINT")),
	}
	if config.AccessKeyID == "" && config.AccessKeySecret == "" && config.Bucket == "" && config.Endpoint == "" {
		return nil, nil
	}
	if config.AccessKeyID == "" || config.AccessKeySecret == "" || config.Bucket == "" || config.Endpoint == "" {
		return nil, errors.New("OSS_ACCESS_KEY_ID, OSS_ACCESS_KEY_SECRET, OSS_BUCKET and OSS_ENDPOINT are required together")
	}
	return NewOSSClient(config), nil
}

func (c *OSSClient) PutFile(ctx context.Context, localPath, key, contentType string) (string, error) {
	if c == nil {
		return "", errors.New("oss client is nil")
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("read file for oss upload: %w", err)
	}
	key = cleanObjectKey(key)
	if key == "" {
		return "", errors.New("oss object key is required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	sum := sha256.Sum256(data)
	metaSHA256 := hex.EncodeToString(sum[:])
	req, err := c.newRequest(ctx, http.MethodPut, key, bytes.NewReader(data), contentType)
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("x-oss-meta-sha256", metaSHA256)
	c.signRequest(req, http.MethodPut, key, contentType)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload oss object: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("upload oss object http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return key, nil
}

func (c *OSSClient) FindObjectBySHA256(ctx context.Context, prefix, hash string) (string, bool, error) {
	if c == nil {
		return "", false, errors.New("oss client is nil")
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", false, nil
	}
	prefix = cleanObjectKey(prefix)
	marker := ""
	for {
		result, err := c.listObjects(ctx, prefix, marker)
		if err != nil {
			return "", false, err
		}
		for _, object := range result.Contents {
			key := strings.TrimSpace(object.Key)
			if key == "" || strings.HasSuffix(key, "/") {
				continue
			}
			remoteHash, exists, err := c.ObjectSHA256(ctx, key)
			if err != nil {
				return "", false, err
			}
			if exists && strings.EqualFold(strings.TrimSpace(remoteHash), hash) {
				return key, true, nil
			}
		}
		if !result.IsTruncated {
			return "", false, nil
		}
		marker = strings.TrimSpace(result.NextMarker)
		if marker == "" && len(result.Contents) > 0 {
			marker = result.Contents[len(result.Contents)-1].Key
		}
		if marker == "" {
			return "", false, nil
		}
	}
}

func (c *OSSClient) ObjectURI(key string) string {
	if c == nil {
		return ""
	}
	key = cleanObjectKey(key)
	if key == "" {
		return ""
	}
	return key
}

func (c *OSSClient) ObjectSHA256(ctx context.Context, key string) (string, bool, error) {
	if c == nil {
		return "", false, errors.New("oss client is nil")
	}
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "s3://") {
		ref, err := ParseS3URI(key)
		if err != nil {
			return "", false, err
		}
		if ref.Bucket != c.config.Bucket {
			return "", false, fmt.Errorf("s3 bucket %q does not match configured bucket %q", ref.Bucket, c.config.Bucket)
		}
		key = ref.Key
	}
	key = cleanObjectKey(key)
	if key == "" {
		return "", false, errors.New("oss object key is required")
	}

	headReq, err := c.newRequest(ctx, http.MethodHead, key, nil, "")
	if err != nil {
		return "", false, err
	}
	headResp, err := c.client.Do(headReq)
	if err != nil {
		return "", false, fmt.Errorf("head oss object: %w", err)
	}
	defer func() { _ = headResp.Body.Close() }()
	if headResp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if headResp.StatusCode < 200 || headResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(headResp.Body, 1024))
		return "", false, fmt.Errorf("head oss object http %d: %s", headResp.StatusCode, strings.TrimSpace(string(body)))
	}
	if hash := strings.TrimSpace(headResp.Header.Get("x-oss-meta-sha256")); hash != "" {
		return hash, true, nil
	}
	// The object exists, but its hash is unknown. Do not GET the body here:
	// active scans call this for every reused object, so fallback downloads
	// turn missing legacy metadata into repeated OSS egress.
	return "", true, nil
}

type listObjectsResult struct {
	XMLName     xml.Name        `xml:"ListBucketResult"`
	IsTruncated bool            `xml:"IsTruncated"`
	NextMarker  string          `xml:"NextMarker"`
	Contents    []objectSummary `xml:"Contents"`
}

type objectSummary struct {
	Key string `xml:"Key"`
}

func (c *OSSClient) listObjects(ctx context.Context, prefix, marker string) (listObjectsResult, error) {
	if err := c.validate(); err != nil {
		return listObjectsResult{}, err
	}
	values := url.Values{}
	if prefix = cleanObjectKey(prefix); prefix != "" {
		values.Set("prefix", prefix)
	}
	if marker = cleanObjectKey(marker); marker != "" {
		values.Set("marker", marker)
	}
	values.Set("max-keys", "1000")
	req, err := c.newBucketRequest(ctx, http.MethodGet, values)
	if err != nil {
		return listObjectsResult{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return listObjectsResult{}, fmt.Errorf("list oss objects: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return listObjectsResult{}, fmt.Errorf("list oss objects http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result listObjectsResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return listObjectsResult{}, fmt.Errorf("decode oss list objects: %w", err)
	}
	return result, nil
}

func (c *OSSClient) DownloadToFile(ctx context.Context, storageKey, destPath string) error {
	if c == nil {
		return errors.New("oss client is nil")
	}
	key, err := c.objectKeyFromStorageKey(storageKey)
	if err != nil {
		return err
	}
	req, err := c.newRequest(ctx, http.MethodGet, key, nil, "")
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("download oss object: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download oss object http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("write downloaded oss object: %w", err)
	}
	return nil
}

func (c *OSSClient) newRequest(ctx context.Context, method, key string, body io.Reader, contentType string) (*http.Request, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	key = cleanObjectKey(key)
	urlText, err := c.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, urlText, body)
	if err != nil {
		return nil, err
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.signRequest(req, method, key, contentType)
	return req, nil
}

func (c *OSSClient) newBucketRequest(ctx context.Context, method string, query url.Values) (*http.Request, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	urlText, err := c.bucketURL()
	if err != nil {
		return nil, err
	}
	if encoded := query.Encode(); encoded != "" {
		urlText += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, method, urlText, nil)
	if err != nil {
		return nil, err
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	canonicalResource := "/" + c.config.Bucket + "/"
	stringToSign := method + "\n\n\n" + date + "\n" + canonicalResource
	mac := hmac.New(sha1.New, []byte(c.config.AccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("Authorization", "OSS "+c.config.AccessKeyID+":"+signature)
	return req, nil
}

func (c *OSSClient) signRequest(req *http.Request, method, key, contentType string) {
	date := req.Header.Get("Date")
	canonicalResource := "/" + c.config.Bucket + "/" + cleanObjectKey(key)
	stringToSign := method + "\n\n" + contentType + "\n" + date + "\n" + canonicalOSSHeaders(req.Header) + canonicalResource
	mac := hmac.New(sha1.New, []byte(c.config.AccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("Authorization", "OSS "+c.config.AccessKeyID+":"+signature)
}

func canonicalOSSHeaders(headers http.Header) string {
	type pair struct {
		key   string
		value string
	}
	pairs := make([]pair, 0)
	for key, values := range headers {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if !strings.HasPrefix(lowerKey, "x-oss-") {
			continue
		}
		trimmedValues := make([]string, 0, len(values))
		for _, value := range values {
			trimmedValues = append(trimmedValues, strings.Join(strings.Fields(value), " "))
		}
		pairs = append(pairs, pair{key: lowerKey, value: strings.Join(trimmedValues, ",")})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].key < pairs[j].key
	})
	var builder strings.Builder
	for _, p := range pairs {
		builder.WriteString(p.key)
		builder.WriteByte(':')
		builder.WriteString(p.value)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func (c *OSSClient) bucketURL() (string, error) {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse oss endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid oss endpoint: %s", endpoint)
	}
	if isLocalEndpoint(parsed.Hostname()) {
		return endpoint + "/" + url.PathEscape(c.config.Bucket) + "/", nil
	}
	host := parsed.Host
	bucketPrefix := strings.ToLower(c.config.Bucket) + "."
	if !strings.HasPrefix(strings.ToLower(parsed.Hostname()), bucketPrefix) {
		if port := parsed.Port(); port != "" {
			host = c.config.Bucket + "." + parsed.Hostname() + ":" + port
		} else {
			host = c.config.Bucket + "." + parsed.Hostname()
		}
	}
	return parsed.Scheme + "://" + host + "/", nil
}

func (c *OSSClient) objectURL(key string) (string, error) {
	endpoint := strings.TrimRight(c.config.Endpoint, "/")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse oss endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid oss endpoint: %s", endpoint)
	}

	keyPath := escapeObjectKey(key)
	basePath := strings.Trim(strings.TrimSpace(parsed.EscapedPath()), "/")
	if basePath != "" {
		keyPath = basePath + "/" + keyPath
	}

	if isLocalEndpoint(parsed.Hostname()) {
		return endpoint + "/" + url.PathEscape(c.config.Bucket) + "/" + keyPath, nil
	}

	host := parsed.Host
	bucketPrefix := strings.ToLower(c.config.Bucket) + "."
	if !strings.HasPrefix(strings.ToLower(parsed.Hostname()), bucketPrefix) {
		if port := parsed.Port(); port != "" {
			host = c.config.Bucket + "." + parsed.Hostname() + ":" + port
		} else {
			host = c.config.Bucket + "." + parsed.Hostname()
		}
	}
	return parsed.Scheme + "://" + host + "/" + keyPath, nil
}

func isLocalEndpoint(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *OSSClient) validate() error {
	if strings.TrimSpace(c.config.AccessKeyID) == "" || strings.TrimSpace(c.config.AccessKeySecret) == "" || strings.TrimSpace(c.config.Bucket) == "" || strings.TrimSpace(c.config.Endpoint) == "" {
		return errors.New("oss config is incomplete")
	}
	return nil
}

func ParseS3URI(value string) (S3Ref, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "s3://") {
		return S3Ref{}, fmt.Errorf("storage key is not s3 uri: %s", value)
	}
	rest := strings.TrimPrefix(value, "s3://")
	bucket, key, ok := strings.Cut(rest, "/")
	if !ok || strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return S3Ref{}, fmt.Errorf("invalid s3 uri: %s", value)
	}
	return S3Ref{Bucket: bucket, Key: cleanObjectKey(key)}, nil
}

func (c *OSSClient) objectKeyFromStorageKey(storageKey string) (string, error) {
	storageKey = strings.TrimSpace(storageKey)
	if strings.HasPrefix(storageKey, "s3://") {
		ref, err := ParseS3URI(storageKey)
		if err != nil {
			return "", err
		}
		if ref.Bucket != c.config.Bucket {
			return "", fmt.Errorf("s3 bucket %q does not match configured bucket %q", ref.Bucket, c.config.Bucket)
		}
		return ref.Key, nil
	}
	key := cleanObjectKey(storageKey)
	if key == "" {
		return "", errors.New("oss object key is required")
	}
	return key, nil
}

func cleanObjectKey(key string) string {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = strings.TrimPrefix(path.Clean("/"+key), "/")
	if key == "." {
		return ""
	}
	return key
}

func escapeObjectKey(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}
