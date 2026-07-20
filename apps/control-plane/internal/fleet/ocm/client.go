package ocm

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/ports"
)

const maxErrorBodyBytes = 4096

var resourceNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

type Config struct {
	HubURL         string
	TokenFile      string
	CAFile         string
	ClientCertFile string
	ClientKeyFile  string
	PollInterval   time.Duration
}

type Client struct {
	baseURL      *url.URL
	httpClient   *http.Client
	pollInterval time.Duration
}

type manifestWorkIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
}

type manifestWorkStatus struct {
	Status struct {
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

type HTTPError struct {
	StatusCode int
	Reason     string
	Body       string
}

func (err *HTTPError) Error() string {
	if err.Reason != "" {
		return fmt.Sprintf("OCM API returned HTTP %d: %s", err.StatusCode, err.Reason)
	}
	return fmt.Sprintf("OCM API returned HTTP %d", err.StatusCode)
}

func NewClient(config Config) (*Client, error) {
	hubURL, err := url.Parse(strings.TrimSpace(config.HubURL))
	if err != nil {
		return nil, fmt.Errorf("parse OCM hub URL: %w", err)
	}
	if hubURL.Scheme != "https" || hubURL.Host == "" || hubURL.User != nil || (hubURL.Path != "" && hubURL.Path != "/") || hubURL.RawQuery != "" || hubURL.Fragment != "" {
		return nil, errors.New("OCM hub URL must be an HTTPS origin")
	}
	hubURL.Path = strings.TrimRight(hubURL.Path, "/")

	if config.PollInterval <= 0 {
		return nil, errors.New("OCM poll interval must be greater than zero")
	}
	if strings.TrimSpace(config.CAFile) == "" {
		return nil, errors.New("OCM CA file is required")
	}
	caPEM, err := os.ReadFile(config.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read OCM CA file: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("OCM CA file does not contain a certificate")
	}

	hasToken := strings.TrimSpace(config.TokenFile) != ""
	hasClientCert := strings.TrimSpace(config.ClientCertFile) != "" || strings.TrimSpace(config.ClientKeyFile) != ""
	if !hasToken && !hasClientCert {
		return nil, errors.New("OCM bearer token file or client certificate is required")
	}
	if (strings.TrimSpace(config.ClientCertFile) == "") != (strings.TrimSpace(config.ClientKeyFile) == "") {
		return nil, errors.New("OCM client certificate and key files must be configured together")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}
	if hasClientCert {
		certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load OCM client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	transport := http.RoundTripper(&http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	})
	if hasToken {
		if _, err := readToken(config.TokenFile); err != nil {
			return nil, err
		}
		transport = &tokenRoundTripper{tokenFile: config.TokenFile, next: transport}
	}

	return &Client{
		baseURL:      hubURL,
		httpClient:   &http.Client{Transport: transport, Timeout: 30 * time.Second},
		pollInterval: config.PollInterval,
	}, nil
}

func (client *Client) ApplyWork(ctx context.Context, request ports.WorkRequest) (ports.WorkResult, error) {
	if !validResourceName(request.ClusterID) || !validResourceName(request.WorkID) {
		return ports.WorkResult{}, errors.New("OCM cluster and work names must be DNS-compatible")
	}
	var identity manifestWorkIdentity
	if err := json.Unmarshal(request.Manifest, &identity); err != nil {
		return ports.WorkResult{}, fmt.Errorf("decode ManifestWork identity: %w", err)
	}
	if identity.APIVersion != "work.open-cluster-management.io/v1" ||
		identity.Kind != "ManifestWork" ||
		identity.Metadata.Name != request.WorkID ||
		identity.Metadata.Namespace != request.ClusterID {
		return ports.WorkResult{}, errors.New("ManifestWork identity does not match the work request")
	}

	endpoint := client.workEndpoint(request.ClusterID, request.WorkID)
	query := endpoint.Query()
	query.Set("fieldManager", "gpu-cloud-control-plane")
	query.Set("force", "true")
	endpoint.RawQuery = query.Encode()

	applyRequest, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint.String(), bytes.NewReader(request.Manifest))
	if err != nil {
		return ports.WorkResult{}, fmt.Errorf("create ManifestWork apply request: %w", err)
	}
	applyRequest.Header.Set("Accept", "application/json")
	applyRequest.Header.Set("Content-Type", "application/apply-patch+yaml")
	response, err := client.httpClient.Do(applyRequest)
	if err != nil {
		return ports.WorkResult{}, fmt.Errorf("apply ManifestWork: %w", err)
	}
	if err := requireSuccess(response); err != nil {
		return ports.WorkResult{}, err
	}

	ticker := time.NewTicker(client.pollInterval)
	defer ticker.Stop()
	for {
		status, err := client.getWork(ctx, request.ClusterID, request.WorkID)
		if err != nil {
			return ports.WorkResult{}, err
		}
		applied, available, degraded, detail := workConditions(status)
		if degraded {
			return ports.WorkResult{}, fmt.Errorf("ManifestWork became degraded: %s", detail)
		}
		if applied && available {
			return ports.WorkResult{WorkID: request.WorkID, Applied: true, Available: true}, nil
		}
		select {
		case <-ctx.Done():
			return ports.WorkResult{}, fmt.Errorf("wait for ManifestWork availability: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (client *Client) Place(context.Context, ports.PlacementRequest) (ports.PlacementResult, error) {
	return ports.PlacementResult{}, errors.New("OCM placement is not enabled in the Phase 1 default-cluster profile")
}

func (client *Client) getWork(ctx context.Context, clusterID, workID string) (manifestWorkStatus, error) {
	endpoint := client.workEndpoint(clusterID, workID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return manifestWorkStatus{}, fmt.Errorf("create ManifestWork status request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return manifestWorkStatus{}, fmt.Errorf("get ManifestWork status: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return manifestWorkStatus{}, responseError(response)
	}
	defer response.Body.Close()
	var status manifestWorkStatus
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return manifestWorkStatus{}, fmt.Errorf("decode ManifestWork status: %w", err)
	}
	return status, nil
}

func (client *Client) workEndpoint(clusterID, workID string) *url.URL {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") +
		"/apis/work.open-cluster-management.io/v1/namespaces/" +
		url.PathEscape(clusterID) + "/manifestworks/" + url.PathEscape(workID)
	return &endpoint
}

func workConditions(status manifestWorkStatus) (bool, bool, bool, string) {
	var applied, available, degraded bool
	var detail string
	for _, condition := range status.Status.Conditions {
		switch condition.Type {
		case "Applied":
			applied = condition.Status == "True"
		case "Available":
			available = condition.Status == "True"
		case "Degraded":
			degraded = condition.Status == "True"
			if condition.Reason != "" || condition.Message != "" {
				detail = strings.TrimSpace(condition.Reason + ": " + condition.Message)
			}
		}
	}
	return applied, available, degraded, detail
}

func requireSuccess(response *http.Response) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return nil
	}
	return responseError(response)
}

func responseError(response *http.Response) error {
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
	var status struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &status)
	reason := strings.TrimSpace(status.Message)
	if reason == "" {
		reason = strings.TrimSpace(status.Reason)
	}
	return &HTTPError{StatusCode: response.StatusCode, Reason: reason, Body: strings.TrimSpace(string(body))}
}

func validResourceName(value string) bool {
	return len(value) > 0 && len(value) <= 253 && resourceNamePattern.MatchString(value)
}

type tokenRoundTripper struct {
	tokenFile string
	next      http.RoundTripper
}

func (transport *tokenRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	token, err := readToken(transport.tokenFile)
	if err != nil {
		return nil, err
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+token)
	return transport.next.RoundTrip(clone)
}

func readToken(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read OCM bearer token file: %w", err)
	}
	token := strings.TrimSpace(string(contents))
	if token == "" {
		return "", errors.New("OCM bearer token file is empty")
	}
	return token, nil
}

var _ ports.FleetManager = (*Client)(nil)
