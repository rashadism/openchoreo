// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/rca/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

const (
	// ActionViewRCAReport is the action for viewing RCA reports.
	ActionViewRCAReport = "rcareport:view"
	// ResourceTypeRCAReport is the resource type for RCA reports.
	ResourceTypeRCAReport = "rcareport"
)

// AuthzClient handles authorization requests.
type AuthzClient struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
	disabled   bool
}

// NewAuthzClient creates a new authorization client.
func NewAuthzClient(cfg *config.Config, logger *slog.Logger) *AuthzClient {
	if !cfg.AuthzEnabled() {
		logger.Info("Authorization is disabled")
		return &AuthzClient{
			disabled: true,
			logger:   logger,
		}
	}

	baseURL := cfg.GetAuthzServiceURL()
	if baseURL == "" {
		logger.Warn("Authorization enabled but no service URL configured, disabling")
		return &AuthzClient{
			disabled: true,
			logger:   logger,
		}
	}

	httpClient := &http.Client{
		Timeout: time.Duration(cfg.AuthzTimeoutSeconds()) * time.Second,
	}

	logger.Info("Authorization client initialized",
		"service_url", baseURL,
		"timeout", cfg.AuthzTimeoutSeconds())

	return &AuthzClient{
		httpClient: httpClient,
		baseURL:    baseURL,
		logger:     logger,
		disabled:   false,
	}
}

// CheckChatAuthorization checks if the user is authorized to access chat for a project.
func (c *AuthzClient) CheckChatAuthorization(ctx context.Context, projectUID, componentUID string) error {
	if c.disabled {
		return nil
	}

	// Get subject context from the request context using existing infrastructure
	authSubjectCtx, ok := auth.GetSubjectContextFromContext(ctx)
	if !ok || authSubjectCtx == nil {
		return fmt.Errorf("no subject context in request")
	}

	// Convert auth.SubjectContext to authz.SubjectContext
	subjectCtx := authzcore.GetAuthzSubjectContext(authSubjectCtx)
	if subjectCtx == nil {
		return fmt.Errorf("failed to convert subject context")
	}

	request := &authzcore.EvaluateRequest{
		SubjectContext: subjectCtx,
		Resource: authzcore.Resource{
			Type: ResourceTypeRCAReport,
			ID:   "",
			Hierarchy: authzcore.ResourceHierarchy{
				Project:   projectUID,
				Component: componentUID,
			},
		},
		Action:  ActionViewRCAReport,
		Context: authzcore.Context{},
	}

	decision, err := c.evaluate(ctx, request)
	if err != nil {
		return fmt.Errorf("authorization check failed: %w", err)
	}

	if !decision.Decision {
		return fmt.Errorf("access denied")
	}

	return nil
}

// evaluate sends an authorization request to the authz service.
func (c *AuthzClient) evaluate(ctx context.Context, request *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.logger.Debug("Authz request", "body", string(body))

	url := c.baseURL + "/api/v1/authz/evaluate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Forward the authentication token
	if token := jwt.GetTokenFromContext(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Authz service request failed", "error", err, "url", url)
		return nil, fmt.Errorf("authz service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized")
	}

	if resp.StatusCode == http.StatusForbidden {
		return &authzcore.Decision{Decision: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.logger.Error("Authz service error", "status", resp.StatusCode, "body", string(bodyBytes))
		return nil, fmt.Errorf("authz service returned %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response struct {
		Success bool               `json:"success"`
		Data    authzcore.Decision `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Debug("Authz decision",
		"action", request.Action,
		"resource_type", request.Resource.Type,
		"decision", response.Data.Decision)

	return &response.Data, nil
}
