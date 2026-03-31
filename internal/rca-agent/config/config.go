// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// Config holds all configuration for the RCA agent service.
type Config struct {
	Server   ServerConfig `koanf:"server"`
	LLM      LLMConfig    `koanf:"llm"`
	MCP      MCPConfig    `koanf:"mcp"`
	Report   ReportConfig `koanf:"report"`
	Auth     AuthConfig   `koanf:"auth"`
	Authz    AuthzConfig  `koanf:"authz"`
	Agent    AgentConfig  `koanf:"agent"`
	LogLevel string       `koanf:"loglevel"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port            int           `koanf:"port"`
	ReadTimeout     time.Duration `koanf:"read.timeout"`
	WriteTimeout    time.Duration `koanf:"write.timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown.timeout"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	ModelName string `koanf:"model.name"`
	APIKey    string `koanf:"api.key"`
}

// MCPConfig holds MCP server connection configuration.
type MCPConfig struct {
	ObserverURL      string `koanf:"observer.url"`
	OpenChoreoAPIURL string `koanf:"openchoreo.api.url"`
}

// ReportConfig holds report storage configuration.
type ReportConfig struct {
	Backend     string `koanf:"backend"`
	DatabaseURI string `koanf:"database.uri"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	JWTDisabled            bool          `koanf:"jwt.disabled"`
	OAuthTokenURL          string        `koanf:"oauth.token.url"`
	OAuthClientID          string        `koanf:"oauth.client.id"`
	OAuthClientSecret      string        `koanf:"oauth.client.secret"`
	JWTJWKSURL             string        `koanf:"jwt.jwks.url"`
	JWTIssuer              string        `koanf:"jwt.issuer"`
	JWTAudience            string        `koanf:"jwt.audience"`
	JWTJWKSRefreshInterval time.Duration `koanf:"jwt.jwks.refresh.interval"`
	ConfigPath             string        `koanf:"config.path"`
	TLSInsecureSkipVerify  bool          `koanf:"tls.insecure.skip.verify"`
	SubjectTypes           []subject.UserTypeConfig // loaded from YAML file
}

// AuthzConfig holds authorization service configuration.
type AuthzConfig struct {
	ServiceURL            string        `koanf:"service.url"`
	Timeout               time.Duration `koanf:"timeout"`
	TLSInsecureSkipVerify bool          `koanf:"tls.insecure.skip.verify"`
}

// AgentConfig holds agent behavior configuration.
type AgentConfig struct {
	MaxConcurrentAnalyses int           `koanf:"max.concurrent.analyses"`
	AnalysisTimeout       time.Duration `koanf:"analysis.timeout"`
	RemediationEnabled    bool          `koanf:"remediation.enabled"`
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string
}

// Load loads configuration from environment variables and defaults.
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load environment variable overrides
	envOverrides := make(map[string]interface{})

	envMappings := map[string]string{
		"SERVER_PORT":              "server.port",
		"SERVER_READ_TIMEOUT":     "server.read.timeout",
		"SERVER_WRITE_TIMEOUT":    "server.write.timeout",
		"SERVER_SHUTDOWN_TIMEOUT": "server.shutdown.timeout",

		"RCA_MODEL_NAME":  "llm.model.name",
		"RCA_LLM_API_KEY": "llm.api.key",

		"OBSERVER_API_URL":    "mcp.observer.url",
		"OPENCHOREO_API_URL":  "mcp.openchoreo.api.url",

		"REPORT_BACKEND":     "report.backend",
		"SQL_BACKEND_URI":    "report.database.uri",

		"JWT_DISABLED":                "auth.jwt.disabled",
		"OAUTH_TOKEN_URL":             "auth.oauth.token.url",
		"OAUTH_CLIENT_ID":             "auth.oauth.client.id",
		"OAUTH_CLIENT_SECRET":         "auth.oauth.client.secret",
		"JWT_JWKS_URL":                "auth.jwt.jwks.url",
		"JWT_ISSUER":                  "auth.jwt.issuer",
		"JWT_AUDIENCE":                "auth.jwt.audience",
		"JWT_JWKS_REFRESH_INTERVAL":   "auth.jwt.jwks.refresh.interval",
		"AUTH_CONFIG_PATH":            "auth.config.path",
		"TLS_INSECURE_SKIP_VERIFY":   "auth.tls.insecure.skip.verify",
		"AUTHZ_SERVICE_URL":              "authz.service.url",
		"AUTHZ_TIMEOUT_SECONDS":          "authz.timeout",
		"AUTHZ_TLS_INSECURE_SKIP_VERIFY": "authz.tls.insecure.skip.verify",

		"MAX_CONCURRENT_ANALYSES":  "agent.max.concurrent.analyses",
		"ANALYSIS_TIMEOUT_SECONDS": "agent.analysis.timeout",
		"REMED_AGENT":             "agent.remediation.enabled",

		"LOG_LEVEL": "loglevel",
	}

	for envKey, configKey := range envMappings {
		if value := os.Getenv(envKey); value != "" {
			parts := strings.Split(configKey, ".")
			if len(parts) == 1 {
				envOverrides[configKey] = value
			} else {
				section := parts[0]
				key := strings.Join(parts[1:], ".")
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = value
			}
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

	// Load auth config file for JWT subject resolution
	authConfigPath := cfg.Auth.ConfigPath
	if authConfigPath == "" {
		authConfigPath = "auth-config.yaml"
	}

	var authCfg struct {
		Auth struct {
			SubjectTypes []subject.UserTypeConfig `yaml:"subject_types"`
		} `yaml:"auth"`
	}
	if _, err := os.Stat(authConfigPath); err == nil {
		data, err := os.ReadFile(authConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read auth config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &authCfg); err != nil {
			return nil, fmt.Errorf("failed to parse auth config file: %w", err)
		}
	}

	cfg.Auth.SubjectTypes = authCfg.Auth.SubjectTypes

	if len(cfg.Auth.SubjectTypes) > 0 {
		if err := subject.ValidateConfig(cfg.Auth.SubjectTypes); err != nil {
			return nil, fmt.Errorf("invalid subject type config: %w", err)
		}
		subject.SortByPriority(cfg.Auth.SubjectTypes)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func getDefaults() map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"port":             8080,
			"read.timeout":    "15s",
			"write.timeout":   "15s",
			"shutdown.timeout": "30s",
		},
		"llm": map[string]interface{}{
			"model.name": "",
			"api.key":    "",
		},
		"mcp": map[string]interface{}{
			"observer.url":      "http://observer:8080",
			"openchoreo.api.url": "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080",
		},
		"report": map[string]interface{}{
			"backend":      "sqlite",
			"database.uri": "sqlite:///data/rca_reports.db",
		},
		"auth": map[string]interface{}{
			"jwt.disabled":               false,
			"oauth.token.url":            "",
			"oauth.client.id":            "",
			"oauth.client.secret":        "",
			"jwt.jwks.url":               "",
			"jwt.issuer":                 "",
			"jwt.audience":               "",
			"jwt.jwks.refresh.interval":  "3600s",
			"config.path":                "auth-config.yaml",
			"tls.insecure.skip.verify":   false,
		},
		"authz": map[string]interface{}{
			"service.url":              "http://localhost:8080",
			"timeout":                  "30s",
			"tls.insecure.skip.verify": false,
		},
		"agent": map[string]interface{}{
			"max.concurrent.analyses": 5,
			"analysis.timeout":        "1500s",
			"remediation.enabled":     false,
		},
		"loglevel": "info",
	}
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.LLM.ModelName == "" {
		return fmt.Errorf("llm model name is required (RCA_MODEL_NAME)")
	}

	if c.LLM.APIKey == "" {
		return fmt.Errorf("llm api key is required (RCA_LLM_API_KEY)")
	}

	c.Report.Backend = strings.ToLower(strings.TrimSpace(c.Report.Backend))
	switch c.Report.Backend {
	case "", "sqlite":
		c.Report.Backend = "sqlite"
		if strings.TrimSpace(c.Report.DatabaseURI) == "" {
			c.Report.DatabaseURI = "sqlite:///data/rca_reports.db"
		}
	case "postgresql":
		if strings.TrimSpace(c.Report.DatabaseURI) == "" {
			return fmt.Errorf("report.database.uri is required when report.backend=postgresql")
		}
	default:
		return fmt.Errorf("report.backend must be 'sqlite' or 'postgresql'")
	}

	if c.Agent.MaxConcurrentAnalyses <= 0 {
		return fmt.Errorf("agent max concurrent analyses must be positive")
	}

	if c.Agent.AnalysisTimeout <= 0 {
		return fmt.Errorf("agent analysis timeout must be positive")
	}

	return nil
}
