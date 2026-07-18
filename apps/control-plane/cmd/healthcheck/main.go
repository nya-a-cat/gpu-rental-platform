package main

import (
	"context"
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
	defaultHealthcheckURL     = "http://127.0.0.1:8080/health/ready"
	defaultHealthcheckTimeout = 2 * time.Second
)

type healthcheckConfig struct {
	URL     string
	Timeout time.Duration
}

func main() {
	if err := run(os.Getenv); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(getenv func(string) string) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: config.Timeout}
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()
	return check(ctx, client, config.URL)
}

func loadConfig(getenv func(string) string) (healthcheckConfig, error) {
	endpoint := strings.TrimSpace(getenv("HEALTHCHECK_URL"))
	if endpoint == "" {
		endpoint = defaultHealthcheckURL
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return healthcheckConfig{}, errors.New("HEALTHCHECK_URL must be an absolute HTTP or HTTPS URL")
	}

	timeout := defaultHealthcheckTimeout
	if value := strings.TrimSpace(getenv("HEALTHCHECK_TIMEOUT")); value != "" {
		timeout, err = time.ParseDuration(value)
		if err != nil {
			return healthcheckConfig{}, fmt.Errorf("HEALTHCHECK_TIMEOUT must be a duration: %w", err)
		}
		if timeout <= 0 {
			return healthcheckConfig{}, errors.New("HEALTHCHECK_TIMEOUT must be greater than zero")
		}
	}
	return healthcheckConfig{URL: endpoint, Timeout: timeout}, nil
}

func check(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}
	request.Header.Set("Accept", "application/json, application/problem+json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("readiness request failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("readiness endpoint returned %s", response.Status)
	}
	return nil
}
