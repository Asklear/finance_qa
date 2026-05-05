package storage

import (
	"context"
	"net/http"
	"testing"
)

func TestOSSClientUsesVirtualHostedEndpointForAliyun(t *testing.T) {
	client := NewOSSClient(OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        "https://oss-cn-shenzhen.aliyuncs.com",
	})

	req, err := client.newRequest(context.Background(), http.MethodGet, "tenant/uhub/contract/a.pdf", nil, "")
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got, want := req.URL.String(), "https://boss-agent.oss-cn-shenzhen.aliyuncs.com/tenant/uhub/contract/a.pdf"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestOSSClientKeepsPathStyleEndpointForLocalhost(t *testing.T) {
	client := NewOSSClient(OSSConfig{
		AccessKeyID:     "test-ak",
		AccessKeySecret: "test-secret",
		Bucket:          "boss-agent",
		Endpoint:        "http://127.0.0.1:9000",
	})

	req, err := client.newRequest(context.Background(), http.MethodGet, "tenant/uhub/contract/a.pdf", nil, "")
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got, want := req.URL.String(), "http://127.0.0.1:9000/boss-agent/tenant/uhub/contract/a.pdf"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}
