package auditarchive

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RuntimeConfig struct {
	DatabaseURL    string
	Endpoint       *url.URL
	AccessKey      string
	SecretKey      string
	Region         string
	Archive        Config
	CommandTimeout time.Duration
}

func LoadRuntimeConfig(getenv func(string) string, now time.Time) (RuntimeConfig, error) {
	databaseURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if databaseURL == "" {
		return RuntimeConfig{}, errors.New("DATABASE_URL is required")
	}
	endpointValue := strings.TrimSpace(getenv("AUDIT_ARCHIVE_S3_ENDPOINT"))
	endpoint, err := url.Parse(endpointValue)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || (endpoint.Path != "" && endpoint.Path != "/") {
		return RuntimeConfig{}, errors.New("AUDIT_ARCHIVE_S3_ENDPOINT must be an http or https origin")
	}
	accessKey := strings.TrimSpace(getenv("AUDIT_ARCHIVE_S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(getenv("AUDIT_ARCHIVE_S3_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		return RuntimeConfig{}, errors.New("AUDIT_ARCHIVE_S3_ACCESS_KEY and AUDIT_ARCHIVE_S3_SECRET_KEY are required")
	}
	bucket := strings.TrimSpace(getenv("AUDIT_ARCHIVE_S3_BUCKET"))
	if bucket == "" {
		return RuntimeConfig{}, errors.New("AUDIT_ARCHIVE_S3_BUCKET is required")
	}
	region := strings.TrimSpace(getenv("AUDIT_ARCHIVE_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	prefix := strings.TrimSpace(getenv("AUDIT_ARCHIVE_PREFIX"))
	if prefix == "" {
		prefix = "audit"
	}
	retentionMode := strings.ToUpper(strings.TrimSpace(getenv("AUDIT_ARCHIVE_RETENTION_MODE")))
	if retentionMode == "" {
		retentionMode = "GOVERNANCE"
	}
	retentionDays, err := positiveInteger(getenv("AUDIT_ARCHIVE_RETENTION_DAYS"), 365)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("AUDIT_ARCHIVE_RETENTION_DAYS: %w", err)
	}
	minimumAge, err := positiveDuration(getenv("AUDIT_ARCHIVE_MIN_AGE"), 7*24*time.Hour)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("AUDIT_ARCHIVE_MIN_AGE: %w", err)
	}
	commandTimeout, err := positiveDuration(getenv("AUDIT_ARCHIVE_TIMEOUT"), 10*time.Minute)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("AUDIT_ARCHIVE_TIMEOUT: %w", err)
	}
	monthStart, err := resolveMonth(strings.TrimSpace(getenv("AUDIT_ARCHIVE_MONTH")), now, minimumAge)
	if err != nil {
		return RuntimeConfig{}, err
	}

	return RuntimeConfig{
		DatabaseURL: databaseURL,
		Endpoint:    endpoint,
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		Region:      region,
		Archive: Config{
			Bucket:        bucket,
			Prefix:        prefix,
			MonthStart:    monthStart,
			RetentionMode: retentionMode,
			RetentionDays: retentionDays,
			TempDir:       strings.TrimSpace(getenv("TMPDIR")),
		},
		CommandTimeout: commandTimeout,
	}, nil
}

func resolveMonth(explicit string, now time.Time, minimumAge time.Duration) (time.Time, error) {
	if explicit != "" {
		month, err := time.Parse("2006-01", explicit)
		if err != nil {
			return time.Time{}, errors.New("AUDIT_ARCHIVE_MONTH must use YYYY-MM")
		}
		return month.UTC(), nil
	}
	cutoff := now.UTC().Add(-minimumAge)
	cutoffMonth := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC)
	return cutoffMonth.AddDate(0, -1, 0), nil
}

func positiveInteger(value string, fallback int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, errors.New("must be a positive integer")
	}
	return parsed, nil
}

func positiveDuration(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, errors.New("must be a positive duration")
	}
	return parsed, nil
}
