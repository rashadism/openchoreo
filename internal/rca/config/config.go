// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"

	observerConfig "github.com/openchoreo/openchoreo/internal/observer/config"
)

// Config holds all configuration for the RCA agent.
type Config struct {
	// LLM settings
	RCAModelName string `koanf:"rca_model_name"`
	RCALLMAPIKey string `koanf:"rca_llm_api_key"`

	// MCP server URLs
	ObserverMCPURL   string `koanf:"observer_mcp_url"`
	OpenchoreoMCPURL string `koanf:"openchoreo_mcp_url"`

	// Logging
	LogLevel string `koanf:"log_level"`

	// OpenSearch config
	OpenSearchAddress  string `koanf:"opensearch_address"`
	OpenSearchUsername string `koanf:"opensearch_username"`
	OpenSearchPassword string `koanf:"opensearch_password"`

	// OAuth2 Client Credentials
	OAuthTokenURL     string `koanf:"oauth_token_url"`
	OAuthClientID     string `koanf:"oauth_client_id"`
	OAuthClientSecret string `koanf:"oauth_client_secret"`

	// Analysis concurrency and timeout settings
	MaxConcurrentAnalyses  int           `koanf:"max_concurrent_analyses"`
	AnalysisTimeoutSeconds int           `koanf:"analysis_timeout_seconds"`
	AnalysisTimeout        time.Duration // Computed from AnalysisTimeoutSeconds

	// TLS settings
	TLSInsecureSkipVerify bool `koanf:"tls_insecure_skip_verify"`

	// Server settings
	ServerPort      int           `koanf:"server_port"`
	ReadTimeout     time.Duration `koanf:"read_timeout"`
	WriteTimeout    time.Duration `koanf:"write_timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`

	// JWT Authentication settings
	JWTDisabled            bool   `koanf:"jwt_disabled"`
	JWTJWKSURL             string `koanf:"jwt_jwks_url"`
	JWTIssuer              string `koanf:"jwt_issuer"`
	JWTAudience            string `koanf:"jwt_audience"`
	JWTJWKSRefreshInterval int    `koanf:"jwt_jwks_refresh_interval"`
	JWKSURLTLSInsecureSkip bool   `koanf:"jwks_url_tls_insecure_skip_verify"`

	// Authorization settings (reuses observer config type)
	Authz           observerConfig.AuthzConfig `koanf:"authz"`
	ControlPlaneURL string                     `koanf:"control_plane_url"`
}

// Load loads configuration from environment variables and defaults.
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Environment variable mappings
	envMappings := map[string]string{
		// LLM
		"RCA_MODEL_NAME":  "rca_model_name",
		"RCA_LLM_API_KEY": "rca_llm_api_key",

		// MCP URLs
		"OBSERVER_MCP_URL":   "observer_mcp_url",
		"OPENCHOREO_MCP_URL": "openchoreo_mcp_url",
		"CONTROL_PLANE_URL":  "control_plane_url",

		// Logging
		"LOG_LEVEL": "log_level",

		// OpenSearch
		"OPENSEARCH_ADDRESS":  "opensearch_address",
		"OPENSEARCH_USERNAME": "opensearch_username",
		"OPENSEARCH_PASSWORD": "opensearch_password",

		// OAuth2
		"OAUTH_TOKEN_URL":     "oauth_token_url",
		"OAUTH_CLIENT_ID":     "oauth_client_id",
		"OAUTH_CLIENT_SECRET": "oauth_client_secret",

		// Analysis settings
		"MAX_CONCURRENT_ANALYSES":  "max_concurrent_analyses",
		"ANALYSIS_TIMEOUT_SECONDS": "analysis_timeout_seconds",

		// TLS
		"TLS_INSECURE_SKIP_VERIFY": "tls_insecure_skip_verify",

		// Server
		"SERVER_PORT":      "server_port",
		"PORT":             "server_port", // Common alias
		"READ_TIMEOUT":     "read_timeout",
		"WRITE_TIMEOUT":    "write_timeout",
		"SHUTDOWN_TIMEOUT": "shutdown_timeout",

		// JWT
		"JWT_DISABLED":                     "jwt_disabled",
		"JWT_JWKS_URL":                     "jwt_jwks_url",
		"JWT_ISSUER":                       "jwt_issuer",
		"JWT_AUDIENCE":                     "jwt_audience",
		"JWT_JWKS_REFRESH_INTERVAL":        "jwt_jwks_refresh_interval",
		"JWKS_URL_TLS_INSECURE_SKIP_VERIFY": "jwks_url_tls_insecure_skip_verify",

		// Authorization
		"AUTHZ_ENABLED":     "authz.enabled",
		"AUTHZ_SERVICE_URL": "authz.service.url",
		"AUTHZ_TIMEOUT":     "authz.timeout",
	}

	// Load environment overrides
	envOverrides := make(map[string]any)
	for envKey, configKey := range envMappings {
		if value := os.Getenv(envKey); value != "" {
			envOverrides[configKey] = value
		}
	}

	if len(envOverrides) > 0 {
		if err := k.Load(confmap.Provider(envOverrides, "."), nil); err != nil {
			return nil, fmt.Errorf("failed to load environment overrides: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Compute derived fields
	cfg.AnalysisTimeout = time.Duration(cfg.AnalysisTimeoutSeconds) * time.Second

	// If OpenchoreoMCPURL not set but ControlPlaneURL is, derive it
	if cfg.OpenchoreoMCPURL == "" && cfg.ControlPlaneURL != "" {
		cfg.OpenchoreoMCPURL = cfg.ControlPlaneURL + "/mcp"
	}

	// If Authz.ServiceURL not set but ControlPlaneURL is, derive it
	if cfg.Authz.ServiceURL == "" && cfg.ControlPlaneURL != "" {
		cfg.Authz.ServiceURL = cfg.ControlPlaneURL
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func getDefaults() map[string]any {
	return map[string]any{
		// LLM defaults
		"rca_model_name":  "",
		"rca_llm_api_key": "",

		// MCP URLs
		"observer_mcp_url":   "http://observer:8080/mcp",
		"openchoreo_mcp_url": "",
		"control_plane_url":  "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080",

		// Logging
		"log_level": "info",

		// OpenSearch
		"opensearch_address":  "https://opensearch:9200",
		"opensearch_username": "admin",
		"opensearch_password": "",

		// OAuth2
		"oauth_token_url":     "",
		"oauth_client_id":     "",
		"oauth_client_secret": "",

		// Analysis settings
		"max_concurrent_analyses":  5,
		"analysis_timeout_seconds": 1200,

		// TLS
		"tls_insecure_skip_verify": false,

		// Server
		"server_port":      8080,
		"read_timeout":     "30s",
		"write_timeout":    "1200s",
		"shutdown_timeout": "10s",

		// JWT
		"jwt_disabled":                      false,
		"jwt_jwks_url":                      "",
		"jwt_issuer":                        "",
		"jwt_audience":                      "",
		"jwt_jwks_refresh_interval":         3600,
		"jwks_url_tls_insecure_skip_verify": false,

		// Authorization
		"authz": map[string]any{
			"enabled":     false,
			"service.url": "http://localhost:8080",
			"timeout":     "30s",
		},
	}
}

func (c *Config) validate() error {
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("invalid server port: %d", c.ServerPort)
	}

	if c.RCALLMAPIKey == "" {
		return fmt.Errorf("RCA_LLM_API_KEY is required")
	}

	if c.MaxConcurrentAnalyses <= 0 {
		return fmt.Errorf("max_concurrent_analyses must be positive")
	}

	if c.AnalysisTimeoutSeconds <= 0 {
		return fmt.Errorf("analysis_timeout_seconds must be positive")
	}

	return nil
}

// IsOAuthConfigured returns true if OAuth credentials are configured.
func (c *Config) IsOAuthConfigured() bool {
	return c.OAuthTokenURL != "" && c.OAuthClientID != "" && c.OAuthClientSecret != ""
}

// GetMCPServers returns the list of MCP server configurations.
func (c *Config) GetMCPServers() []MCPServerConfig {
	var servers []MCPServerConfig

	if c.ObserverMCPURL != "" {
		servers = append(servers, MCPServerConfig{
			Name: "observability",
			URL:  c.ObserverMCPURL,
		})
	}

	if c.OpenchoreoMCPURL != "" {
		servers = append(servers, MCPServerConfig{
			Name: "openchoreo",
			URL:  c.OpenchoreoMCPURL,
		})
	}

	return servers
}

// MCPServerConfig holds configuration for a single MCP server.
type MCPServerConfig struct {
	Name string
	URL  string
}

// AuthzEnabled returns whether authorization is enabled.
func (c *Config) AuthzEnabled() bool {
	return c.Authz.Enabled
}

// GetAuthzServiceURL returns the authorization service URL.
func (c *Config) GetAuthzServiceURL() string {
	return c.Authz.ServiceURL
}

// AuthzTimeoutSeconds returns the authorization timeout in seconds.
func (c *Config) AuthzTimeoutSeconds() int {
	return int(c.Authz.Timeout.Seconds())
}
