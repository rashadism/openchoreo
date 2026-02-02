// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultTimeout = 30 * time.Second

// Config represents configuration for an MCP server.
type Config struct {
	Name          string
	URL           string
	Headers       map[string]string
	TLSSkipVerify bool
	Transport     http.RoundTripper // Optional custom transport (e.g., for OAuth)
}

// Manager manages multiple MCP client connections.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*gomcp.ClientSession
	configs  map[string]Config
	logger   *slog.Logger
}

// NewManager creates a new MCP manager.
// If logger is nil, slog.Default() will be used.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		sessions: make(map[string]*gomcp.ClientSession),
		configs:  make(map[string]Config),
		logger:   logger,
	}
}

// Initialize connects to all configured MCP servers concurrently.
// Returns an error if any connection fails.
func (m *Manager) Initialize(ctx context.Context, configs []Config) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(configs))

	for _, cfg := range configs {
		m.configs[cfg.Name] = cfg

		wg.Add(1)
		go func(cfg Config) {
			defer wg.Done()
			if err := m.connect(ctx, cfg); err != nil {
				errs <- fmt.Errorf("%s: %w", cfg.Name, err)
			}
		}(cfg)
	}

	wg.Wait()
	close(errs)

	var connErrors []error
	for err := range errs {
		connErrors = append(connErrors, err)
	}

	if len(connErrors) > 0 {
		return errors.Join(connErrors...)
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	session, err := m.createSession(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// List tools to verify connection
	tools, err := session.ListTools(ctx, &gomcp.ListToolsParams{})
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to list tools: %w", err)
	}

	m.mu.Lock()
	m.sessions[cfg.Name] = session
	m.mu.Unlock()

	m.logger.Info("MCP connected", "server", cfg.Name, "tools", len(tools.Tools))
	return nil
}

func (m *Manager) createSession(ctx context.Context, cfg Config) (*gomcp.ClientSession, error) {
	httpTransport := cfg.Transport
	if httpTransport == nil {
		httpTransport = newTransport(cfg.TLSSkipVerify)
	}

	// Wrap with header injector if static headers are configured
	if len(cfg.Headers) > 0 {
		httpTransport = &headerRoundTripper{
			Headers:   cfg.Headers,
			Transport: httpTransport,
		}
	}

	httpClient := &http.Client{
		Transport: httpTransport,
	}

	mcpTransport := &gomcp.StreamableClientTransport{
		Endpoint:   cfg.URL,
		HTTPClient: httpClient,
	}

	client := gomcp.NewClient(
		&gomcp.Implementation{
			Name:    "openchoreo-rca-agent",
			Version: "1.0.0",
		},
		nil,
	)

	return client.Connect(ctx, mcpTransport, nil)
}

// GetSession returns a session, reconnecting if necessary.
func (m *Manager) GetSession(ctx context.Context, name string) (*gomcp.ClientSession, error) {
	m.mu.RLock()
	session, ok := m.sessions[name]
	cfg, hasCfg := m.configs[name]
	m.mu.RUnlock()

	if !hasCfg {
		return nil, fmt.Errorf("mcp '%s' not configured", name)
	}

	// Check if session is alive
	if ok {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := session.Ping(pingCtx, nil); err == nil {
			return session, nil
		}
		// Ping failed - close and remove stale session before reconnecting
		session.Close()
		m.mu.Lock()
		delete(m.sessions, name)
		m.mu.Unlock()
	}

	// Reconnect with fresh session
	m.logger.Debug("MCP reconnecting", "server", name)

	reconnectCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	newSession, err := m.createSession(reconnectCtx, cfg)
	if err != nil {
		return nil, fmt.Errorf("reconnection failed: %w", err)
	}

	m.mu.Lock()
	m.sessions[name] = newSession
	m.mu.Unlock()

	m.logger.Info("MCP reconnected", "server", name)
	return newSession, nil
}

// GetAllTools returns all tools from connected MCP servers.
func (m *Manager) GetAllTools(ctx context.Context) []*Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []*Tool

	for name, session := range m.sessions {
		listed, err := session.ListTools(ctx, &gomcp.ListToolsParams{})
		if err != nil {
			m.logger.Warn("MCP failed to list tools", "server", name, "error", err)
			continue
		}

		for _, tool := range listed.Tools {
			tools = append(tools, &Tool{
				manager:    m,
				serverName: name,
				tool:       tool,
			})
		}
	}

	return tools
}

// Close closes all MCP client sessions.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, session := range m.sessions {
		if err := session.Close(); err != nil &&
			!errors.Is(err, io.EOF) &&
			!errors.Is(err, context.Canceled) {
			m.logger.Warn("MCP close error", "server", name, "error", err)
		}
	}

	return nil
}

// newTransport creates an http.RoundTripper with optional TLS skip verification.
func newTransport(skipTLSVerify bool) http.RoundTripper {
	if skipTLSVerify {
		return &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	return http.DefaultTransport
}

// headerRoundTripper wraps a transport to inject headers into all requests.
type headerRoundTripper struct {
	Headers   map[string]string
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range rt.Headers {
		req.Header.Set(k, v)
	}
	return rt.Transport.RoundTrip(req)
}
