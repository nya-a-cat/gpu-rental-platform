package auditarchive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestS3StoreUsesObjectLockAndMetadata(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	var metadata http.Header
	var retentionMode string
	var retentionUntil string
	var authorization string

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Query().Has("location") {
			response.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(response, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		if request.URL.Path != "/audit/platform/year=2026/month=06/audit-events-2026-06.jsonl" {
			http.NotFound(response, request)
			return
		}

		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodHead:
			if body == nil {
				response.Header().Set("Content-Type", "application/xml")
				response.Header().Set("x-amz-request-id", "fixture-request")
				response.WriteHeader(http.StatusNotFound)
				return
			}
			response.Header().Set("Content-Length", strconv.Itoa(len(body)))
			response.Header().Set("Last-Modified", "Mon, 20 Jul 2026 04:00:00 GMT")
			for key, values := range metadata {
				for _, value := range values {
					response.Header().Add(key, value)
				}
			}
			response.Header().Set("ETag", `"fixture-etag"`)
		case http.MethodPut:
			body, _ = io.ReadAll(request.Body)
			metadata = request.Header.Clone()
			retentionMode = request.Header.Get("X-Amz-Object-Lock-Mode")
			retentionUntil = request.Header.Get("X-Amz-Object-Lock-Retain-Until-Date")
			authorization = request.Header.Get("Authorization")
			response.Header().Set("ETag", `"fixture-etag"`)
			response.WriteHeader(http.StatusOK)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	store, err := NewS3Store(S3Config{
		Endpoint:  endpoint,
		AccessKey: "fixture-access",
		SecretKey: "fixture-secret",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("NewS3Store() error = %v", err)
	}
	now := time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC)
	result, err := Archive(context.Background(), Config{
		Bucket:        "audit",
		Prefix:        "platform",
		MonthStart:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		RetentionMode: "GOVERNANCE",
		RetentionDays: 365,
		TempDir:       t.TempDir(),
	}, staticSource{data: "{\"id\":\"one\"}\n", rows: 1}, store, now)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if result.Status != "uploaded" {
		t.Fatalf("status = %q, want uploaded", result.Status)
	}
	if retentionMode != "GOVERNANCE" {
		t.Fatalf("retention mode = %q", retentionMode)
	}
	if retentionUntil == "" {
		t.Fatal("retention date is empty")
	}
	if metadata.Get("X-Amz-Meta-Sha256") != result.SHA256 || metadata.Get("X-Amz-Meta-Row-Count") != "1" {
		t.Fatalf("archive metadata = %#v", metadata)
	}
	if !strings.HasPrefix(authorization, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("authorization = %q", authorization)
	}
}
