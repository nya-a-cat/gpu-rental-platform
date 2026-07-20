package auditarchive

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
			var uploaded []byte
			var err error
			if strings.HasPrefix(request.Header.Get("X-Amz-Content-Sha256"), "STREAMING-AWS4-HMAC-SHA256-PAYLOAD") {
				uploaded, err = readStreamingV4Body(request.Body)
			} else {
				uploaded, err = io.ReadAll(request.Body)
			}
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			body = uploaded
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
	if string(body) != "{\"id\":\"one\"}\n" {
		t.Fatalf("uploaded body = %q", body)
	}
}

func readStreamingV4Body(reader io.Reader) ([]byte, error) {
	buffered := bufio.NewReader(reader)
	var decoded bytes.Buffer
	for {
		header, err := buffered.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read streaming chunk header: %w", err)
		}
		header = strings.TrimSuffix(header, "\r\n")
		size, err := strconv.ParseInt(strings.SplitN(header, ";", 2)[0], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parse streaming chunk size: %w", err)
		}
		if size > 0 {
			if _, err := io.CopyN(&decoded, buffered, size); err != nil {
				return nil, fmt.Errorf("read streaming chunk: %w", err)
			}
		}
		terminator := make([]byte, 2)
		if _, err := io.ReadFull(buffered, terminator); err != nil {
			return nil, fmt.Errorf("read streaming chunk terminator: %w", err)
		}
		if string(terminator) != "\r\n" {
			return nil, fmt.Errorf("invalid streaming chunk terminator: %q", terminator)
		}
		if size == 0 {
			if buffered.Buffered() != 0 {
				return nil, fmt.Errorf("unexpected streaming chunk trailer")
			}
			return decoded.Bytes(), nil
		}
	}
}
