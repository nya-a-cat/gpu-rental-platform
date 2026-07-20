package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nya-a-cat/gpu-rental-platform/apps/control-plane/internal/clusterstate"
)

const (
	defaultHTTPAddr                  = ":8080"
	defaultShutdownTimeout           = 15 * time.Second
	defaultReadinessTimeout          = 2 * time.Second
	defaultMaxOpenConns              = 20
	defaultMaxIdleConns              = 5
	defaultConnMaxLifetime           = 30 * time.Minute
	defaultMigrationTimeout          = 5 * time.Minute
	defaultMigrationLockTimeout      = 30 * time.Second
	defaultMigrationStatementTimeout = 2 * time.Minute
	defaultControlPlaneName          = "gpu-cloud-control-plane"
	defaultControlPlaneStage         = "phase-1-shared-isolation"
	defaultBreakGlassAdminSubject    = "break-glass-admin"
	defaultOCMHubURL                 = "https://kubernetes.default.svc"
	defaultOCMCAFile                 = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	defaultOCMTokenFile              = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultOCMAddonInstallNamespace  = "open-cluster-management-agent-addon"
	defaultOCMAddonServiceAccount    = "gpu-platform-addon-agent"
	defaultOCMReconcileTimeout       = 2 * time.Minute
	defaultOCMPollInterval           = 2 * time.Second
	defaultOCMMaxAttempts            = 8
)

var ocmResourceNamePattern = regexp.MustCompile("^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$")

type OCMConfig struct {
	Enabled               bool
	HubURL                string
	DefaultClusterID      string
	TokenFile             string
	CAFile                string
	ClientCertFile        string
	ClientKeyFile         string
	AddonInstallNamespace string
	AddonServiceAccount   string
	ReconcileTimeout      time.Duration
	PollInterval          time.Duration
	MaxAttempts           int
}

// Config contains the process configuration shared by the API and migration
// commands. DATABASE_URL is intentionally mandatory so a process cannot start
// against an accidental in-memory or simulated persistence layer.
type Config struct {
	HTTPAddr                  string
	DatabaseURL               string
	Version                   string
	Commit                    string
	ShutdownTimeout           time.Duration
	ReadinessTimeout          time.Duration
	BreakGlassAdminToken      string
	BreakGlassAdminSubject    string
	AgentHealthPolicy         clusterstate.Thresholds
	OCM                       OCMConfig
	MaxOpenConns              int
	MaxIdleConns              int
	ConnMaxLifetime           time.Duration
	MigrationTimeout          time.Duration
	MigrationLockTimeout      time.Duration
	MigrationStatementTimeout time.Duration
	ServiceName               string
	Stage                     string
}

func Load() (Config, error) {
	httpAddr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if httpAddr == "" {
		// CONTROL_PLANE_ADDR is retained as a short compatibility bridge for
		// early Phase 0 manifests. New deployments use HTTP_ADDR.
		httpAddr = strings.TrimSpace(os.Getenv("CONTROL_PLANE_ADDR"))
	}
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	breakGlassAdminToken := strings.TrimSpace(os.Getenv("BREAK_GLASS_ADMIN_TOKEN"))
	breakGlassAdminSubject := stringEnv("BREAK_GLASS_ADMIN_SUBJECT", defaultBreakGlassAdminSubject)
	if breakGlassAdminToken != "" && len(breakGlassAdminToken) < 32 {
		return Config{}, errors.New("BREAK_GLASS_ADMIN_TOKEN must contain at least 32 characters")
	}
	if len(breakGlassAdminSubject) > 255 {
		return Config{}, errors.New("BREAK_GLASS_ADMIN_SUBJECT must contain at most 255 characters")
	}

	shutdownTimeout, err := durationEnv("SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	readinessTimeout, err := durationEnv("READINESS_TIMEOUT", defaultReadinessTimeout)
	if err != nil {
		return Config{}, err
	}
	agentHealthPolicy := clusterstate.DefaultThresholds()
	agentHealthPolicy.HeartbeatInterval, err = durationEnv("AGENT_HEARTBEAT_INTERVAL", agentHealthPolicy.HeartbeatInterval)
	if err != nil {
		return Config{}, err
	}
	agentHealthPolicy.DegradedAfter, err = durationEnv("AGENT_DEGRADED_AFTER", agentHealthPolicy.DegradedAfter)
	if err != nil {
		return Config{}, err
	}
	agentHealthPolicy.OfflineAfter, err = durationEnv("AGENT_OFFLINE_AFTER", agentHealthPolicy.OfflineAfter)
	if err != nil {
		return Config{}, err
	}
	if err := agentHealthPolicy.Validate(); err != nil {
		return Config{}, err
	}

	ocmEnabled, err := boolEnv("OCM_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	ocmConfig := OCMConfig{
		Enabled:               ocmEnabled,
		HubURL:                stringEnv("OCM_HUB_URL", defaultOCMHubURL),
		DefaultClusterID:      strings.TrimSpace(os.Getenv("OCM_DEFAULT_CLUSTER_ID")),
		CAFile:                stringEnv("OCM_CA_FILE", defaultOCMCAFile),
		ClientCertFile:        strings.TrimSpace(os.Getenv("OCM_CLIENT_CERT_FILE")),
		ClientKeyFile:         strings.TrimSpace(os.Getenv("OCM_CLIENT_KEY_FILE")),
		AddonInstallNamespace: stringEnv("OCM_ADDON_INSTALL_NAMESPACE", defaultOCMAddonInstallNamespace),
		AddonServiceAccount:   stringEnv("OCM_ADDON_SERVICE_ACCOUNT", defaultOCMAddonServiceAccount),
	}
	if ocmConfig.ClientCertFile == "" && ocmConfig.ClientKeyFile == "" {
		ocmConfig.TokenFile = stringEnv("OCM_TOKEN_FILE", defaultOCMTokenFile)
	} else {
		ocmConfig.TokenFile = strings.TrimSpace(os.Getenv("OCM_TOKEN_FILE"))
	}
	ocmConfig.ReconcileTimeout, err = durationEnv("OCM_RECONCILE_TIMEOUT", defaultOCMReconcileTimeout)
	if err != nil {
		return Config{}, err
	}
	ocmConfig.PollInterval, err = durationEnv("OCM_POLL_INTERVAL", defaultOCMPollInterval)
	if err != nil {
		return Config{}, err
	}
	ocmConfig.MaxAttempts, err = positiveIntEnv("OCM_MAX_ATTEMPTS", defaultOCMMaxAttempts)
	if err != nil {
		return Config{}, err
	}
	if err := validateOCMConfig(ocmConfig); err != nil {
		return Config{}, err
	}

	maxOpenConns, err := positiveIntEnv("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	if err != nil {
		return Config{}, err
	}
	maxIdleConns, err := nonNegativeIntEnv("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)
	if err != nil {
		return Config{}, err
	}
	if maxIdleConns > maxOpenConns {
		return Config{}, errors.New("DB_MAX_IDLE_CONNS must be less than or equal to DB_MAX_OPEN_CONNS")
	}
	connMaxLifetime, err := durationEnv("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)
	if err != nil {
		return Config{}, err
	}
	migrationTimeout, err := millisecondDurationEnv("MIGRATION_TIMEOUT", defaultMigrationTimeout)
	if err != nil {
		return Config{}, err
	}
	migrationLockTimeout, err := millisecondDurationEnv("MIGRATION_LOCK_TIMEOUT", defaultMigrationLockTimeout)
	if err != nil {
		return Config{}, err
	}
	migrationStatementTimeout, err := millisecondDurationEnv("MIGRATION_STATEMENT_TIMEOUT", defaultMigrationStatementTimeout)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:                  httpAddr,
		DatabaseURL:               databaseURL,
		Version:                   stringEnv("CONTROL_PLANE_VERSION", "dev"),
		Commit:                    stringEnv("CONTROL_PLANE_COMMIT", "unknown"),
		ShutdownTimeout:           shutdownTimeout,
		ReadinessTimeout:          readinessTimeout,
		BreakGlassAdminToken:      breakGlassAdminToken,
		BreakGlassAdminSubject:    breakGlassAdminSubject,
		AgentHealthPolicy:         agentHealthPolicy,
		OCM:                       ocmConfig,
		MaxOpenConns:              maxOpenConns,
		MaxIdleConns:              maxIdleConns,
		ConnMaxLifetime:           connMaxLifetime,
		MigrationTimeout:          migrationTimeout,
		MigrationLockTimeout:      migrationLockTimeout,
		MigrationStatementTimeout: migrationStatementTimeout,
		ServiceName:               defaultControlPlaneName,
		Stage:                     defaultControlPlaneStage,
	}, nil
}

func stringEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return parsed, nil
}

func millisecondDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value, err := durationEnv(name, fallback)
	if err != nil {
		return 0, err
	}
	if value < time.Millisecond {
		return 0, fmt.Errorf("%s must be at least 1ms", name)
	}
	return value, nil
}

func positiveIntEnv(name string, fallback int) (int, error) {
	value, err := nonNegativeIntEnv(name, fallback)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return value, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}

func validateOCMConfig(config OCMConfig) error {
	if !config.Enabled {
		return nil
	}
	hubURL, err := url.Parse(config.HubURL)
	if err != nil || hubURL.Scheme != "https" || hubURL.Host == "" || hubURL.User != nil || (hubURL.Path != "" && hubURL.Path != "/") || hubURL.RawQuery != "" || hubURL.Fragment != "" {
		return errors.New("OCM_HUB_URL must be an HTTPS origin")
	}
	for name, value := range map[string]string{
		"OCM_DEFAULT_CLUSTER_ID":      config.DefaultClusterID,
		"OCM_ADDON_INSTALL_NAMESPACE": config.AddonInstallNamespace,
		"OCM_ADDON_SERVICE_ACCOUNT":   config.AddonServiceAccount,
	} {
		if len(value) == 0 || len(value) > 253 || !ocmResourceNamePattern.MatchString(value) {
			return fmt.Errorf("%s must be a DNS-compatible resource name", name)
		}
	}
	if config.ReconcileTimeout <= config.PollInterval {
		return errors.New("OCM_RECONCILE_TIMEOUT must be greater than OCM_POLL_INTERVAL")
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return errors.New("OCM_CLIENT_CERT_FILE and OCM_CLIENT_KEY_FILE must be configured together")
	}
	if config.TokenFile == "" && config.ClientCertFile == "" {
		return errors.New("OCM_TOKEN_FILE or OCM client certificate credentials are required")
	}
	return nil
}

func nonNegativeIntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must be non-negative", name)
	}
	return parsed, nil
}
