// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mozilla-ai/any-llm-go/providers"

	"github.com/openchoreo/openchoreo/internal/agent"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
	"github.com/openchoreo/openchoreo/internal/rca-agent/models"
	"github.com/openchoreo/openchoreo/internal/rca-agent/prompts"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
)

// AnalysisParams holds the parameters for RunAnalysis.
// Scope must be resolved before calling RunAnalysis (matching Python's pattern
// where scope resolution happens in the HTTP handler).
type AnalysisParams struct {
	ReportID    string
	AlertID     string
	Scope       *Scope
	Alert       AlertParams
	Meta        map[string]any
}

// AlertParams describes the alert that triggered the analysis.
type AlertParams struct {
	ID        string
	Value     float64
	Timestamp string
	Rule      AlertRuleParams
}

// AlertRuleParams describes the alert rule.
type AlertRuleParams struct {
	Name        string
	Description string
	Severity    string
	Source      *AlertSourceParams
	Condition   *AlertConditionParams
}

// AlertSourceParams describes the alert source.
type AlertSourceParams struct {
	Type   string
	Query  string
	Metric string
}

// AlertConditionParams describes the alert condition.
type AlertConditionParams struct {
	Window    string
	Interval  string
	Operator  string
	Threshold int
}

// ChatParams holds the parameters for StreamChat.
type ChatParams struct {
	Messages      []ChatMessage
	ReportContext any
	Scope         *prompts.Scope
}

// ChatMessage is a single message in a chat conversation.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatEvent is a streaming event emitted by StreamChat.
type ChatEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Actions []any  `json:"actions,omitempty"`
	Message string `json:"message,omitempty"`
}

// Service orchestrates RCA agent operations: analysis and chat.
type Service struct {
	provider providers.Provider
	config   *config.Config
	store    store.ReportStore
	logger   *slog.Logger
	sem      chan struct{}
}

// New creates a new agent service.
func New(provider providers.Provider, cfg *config.Config, reportStore store.ReportStore, logger *slog.Logger) *Service {
	return &Service{
		provider: provider,
		config:   cfg,
		store:    reportStore,
		logger:   logger,
		sem:      make(chan struct{}, cfg.Agent.MaxConcurrentAnalyses),
	}
}

// ResolveComponentScope resolves namespace/project/component/environment names
// to UIDs via the OpenChoreo API. Called from the HTTP handler before creating
// the pending report, matching Python's pattern.
func (s *Service) ResolveComponentScope(ctx context.Context, namespace, project, component, environment string) (*Scope, error) {
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)
	return resolveComponentScope(ctx, s.config.MCP.OpenChoreoAPIURL, httpClient, namespace, project, component, environment)
}

// ResolveProjectScope resolves namespace/project/environment names to UIDs.
// Used by ListReports and Chat handlers.
func (s *Service) ResolveProjectScope(ctx context.Context, namespace, project, environment string) (*Scope, error) {
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)
	return resolveProjectScope(ctx, s.config.MCP.OpenChoreoAPIURL, httpClient, namespace, project, environment)
}

// RunAnalysis runs the RCA agent. Intended to be called as a goroutine.
// It acquires a semaphore slot, creates an agent, runs analysis, and stores
// the result. On any failure the report is marked as "failed".
func (s *Service) RunAnalysis(ctx context.Context, params *AnalysisParams) {
	logger := s.logger.With("report_id", params.ReportID, "alert_id", params.AlertID)

	// Acquire semaphore.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		s.markFailed(ctx, params, "analysis cancelled while waiting for slot", logger)
		return
	}

	// Apply timeout.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.config.Agent.AnalysisTimeout)*time.Second)
	defer cancel()

	scope := params.Scope
	report, err := s.runRCAAgent(ctx, params, logger)
	if err != nil {
		summary := fmt.Sprintf("Analysis failed: %v", err)
		if ctx.Err() != nil {
			summary = fmt.Sprintf("Analysis timed out (report_id: %s)", params.ReportID)
		}
		logger.Error("analysis failed", "error", err)
		s.markFailed(context.Background(), params, summary, logger)
		return
	}

	// Store completed report.
	reportJSON, err := json.Marshal(report)
	if err != nil {
		logger.Error("failed to marshal report", "error", err)
		s.markFailed(context.Background(), params, "failed to marshal report", logger)
		return
	}

	reportStr := string(reportJSON)
	summaryPtr := extractSummary(report)
	if err := s.store.UpsertReport(context.Background(), &store.ReportEntry{
		ReportID:       params.ReportID,
		AlertID:        params.AlertID,
		Status:         "completed",
		Summary:        summaryPtr,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		EnvironmentUID: scope.EnvironmentUID,
		ProjectUID:     scope.ProjectUID,
		Report:         &reportStr,
	}); err != nil {
		logger.Error("failed to store completed report", "error", err)
	}

	logger.Info("analysis completed")
}

// StreamChat streams chat agent events. The bearerToken is the user's JWT,
// used to authenticate MCP server requests on behalf of the user.
func (s *Service) StreamChat(ctx context.Context, bearerToken string, params *ChatParams) (<-chan ChatEvent, <-chan error) {
	events := make(chan ChatEvent, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		if err := s.runChatAgent(ctx, bearerToken, params, events); err != nil {
			errs <- err
		}
	}()

	return events, errs
}

func (s *Service) runRCAAgent(ctx context.Context, params *AnalysisParams, logger *slog.Logger) (json.RawMessage, error) {
	scope := params.Scope

	// Fetch fresh OAuth2 token for MCP connections.
	logger.Info("fetching OAuth2 token for MCP", "token_url", s.config.Auth.OAuthTokenURL)
	token, err := fetchOAuth2Token(ctx,
		s.config.Auth.OAuthTokenURL,
		s.config.Auth.OAuthClientID,
		s.config.Auth.OAuthClientSecret,
		s.config.Auth.TLSInsecureSkipVerify,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching OAuth2 token: %w", err)
	}
	logger.Info("OAuth2 token acquired")

	httpClient := authedHTTPClient(token, s.config.Auth.TLSInsecureSkipVerify)

	logger.Info("connecting to MCP servers")
	tools, cleanup, err := s.loadMCPTools(ctx, httpClient)
	if err != nil {
		return nil, fmt.Errorf("loading MCP tools: %w", err)
	}
	defer cleanup()
	logger.Info("MCP tools loaded", "count", len(tools))

	obsTools, ocTools := classifyTools(tools)

	systemPrompt, err := prompts.RenderRCAPrompt(&prompts.RCAPromptData{
		ObservabilityTools: obsTools,
		OpenchoreoTools:    ocTools,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering RCA prompt: %w", err)
	}

	promptScope := &prompts.Scope{
		Namespace:   scope.Namespace,
		Environment: scope.Environment,
		Project:     scope.Project,
		Component:   scope.Component,
	}
	userMessage, err := prompts.RenderRCARequest(&prompts.RCARequestData{
		Scope: promptScope,
		Alert: toPromptAlert(&params.Alert),
		Meta:  params.Meta,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering RCA request: %w", err)
	}

	rcaSchema, err := models.RCAReportSchema()
	if err != nil {
		return nil, fmt.Errorf("loading RCA schema: %w", err)
	}

	logger.Info("creating RCA agent", "model", s.config.LLM.ModelName, "tools", len(tools))
	rcaAgent, err := agent.CreateAgent(s.provider, s.config.LLM.ModelName,
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(tools...),
		agent.WithStructuredOutput(&agent.StructuredOutput{
			Strategy: agent.OutputStrategyProvider,
			Name:     "rca_report",
			Schema:   rcaSchema,
		}),
		agent.WithMaxIterations(200),
		agent.WithLogger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	logger.Info("running RCA agent")
	result, err := rcaAgent.Run(ctx, []providers.Message{
		{Role: providers.RoleUser, Content: userMessage},
	})
	if err != nil {
		return nil, fmt.Errorf("running agent: %w", err)
	}

	if result.StructuredResponse == nil {
		return nil, fmt.Errorf("agent did not produce structured output")
	}

	return result.StructuredResponse, nil
}

func (s *Service) runChatAgent(ctx context.Context, bearerToken string, params *ChatParams, events chan<- ChatEvent) error {
	// Chat uses the user's bearer token for MCP auth.
	httpClient := authedHTTPClient(bearerToken, s.config.Auth.TLSInsecureSkipVerify)

	tools, cleanup, err := s.loadMCPTools(ctx, httpClient)
	if err != nil {
		return fmt.Errorf("loading MCP tools: %w", err)
	}
	defer cleanup()

	obsTools, ocTools := classifyTools(tools)

	systemPrompt, err := prompts.RenderChatPrompt(&prompts.ChatPromptData{
		ObservabilityTools: obsTools,
		OpenchoreoTools:    ocTools,
		Scope:              params.Scope,
		ReportContext:      params.ReportContext,
	})
	if err != nil {
		return fmt.Errorf("rendering chat prompt: %w", err)
	}

	chatSchema, err := models.ChatResponseSchema()
	if err != nil {
		return fmt.Errorf("loading chat schema: %w", err)
	}

	chatAgent, err := agent.CreateAgent(s.provider, s.config.LLM.ModelName,
		agent.WithSystemPrompt(systemPrompt),
		agent.WithTools(tools...),
		agent.WithStructuredOutput(&agent.StructuredOutput{
			Strategy: agent.OutputStrategyProvider,
			Name:     "chat_response",
			Schema:   chatSchema,
		}),
		agent.WithMaxIterations(50),
		agent.WithLogger(s.logger),
	)
	if err != nil {
		return fmt.Errorf("creating chat agent: %w", err)
	}

	msgs := make([]providers.Message, len(params.Messages))
	for i, m := range params.Messages {
		msgs[i] = providers.Message{Role: m.Role, Content: m.Content}
	}

	agentEvents, agentErrs := chatAgent.Stream(ctx, msgs)

	for ev := range agentEvents {
		switch ev.Type {
		case agent.StreamEventTextDelta:
			events <- ChatEvent{Type: "message_chunk", Content: ev.Delta}
		case agent.StreamEventToolCallStart:
			events <- ChatEvent{Type: "tool_call", Tool: ev.ToolName}
		case agent.StreamEventToolResult:
			// Internal; not forwarded to client.
		case agent.StreamEventComplete:
			if ev.Result != nil && ev.Result.StructuredResponse != nil {
				var parsed struct {
					Message string `json:"message"`
					Actions []any  `json:"actions"`
				}
				if err := json.Unmarshal(ev.Result.StructuredResponse, &parsed); err == nil {
					if len(parsed.Actions) > 0 {
						events <- ChatEvent{Type: "actions", Actions: parsed.Actions}
					}
					events <- ChatEvent{Type: "done", Message: parsed.Message}
				} else {
					events <- ChatEvent{Type: "done", Message: string(ev.Result.StructuredResponse)}
				}
			} else {
				events <- ChatEvent{Type: "done"}
			}
		}
	}

	return <-agentErrs
}

func (s *Service) loadMCPTools(ctx context.Context, httpClient *http.Client) ([]agent.Tool, func(), error) {
	mcpClient := agent.NewMCPClient([]agent.MCPServer{
		{Name: "observer", URL: strings.TrimRight(s.config.MCP.ObserverURL, "/") + "/mcp", HTTPClient: httpClient},
		{Name: "openchoreo", URL: strings.TrimRight(s.config.MCP.OpenChoreoAPIURL, "/") + "/mcp", HTTPClient: httpClient},
	}, agent.WithMCPLogger(s.logger))

	if err := mcpClient.Connect(ctx); err != nil {
		return nil, nil, err
	}

	tools, err := mcpClient.Tools(ctx)
	if err != nil {
		_ = mcpClient.Close()
		return nil, nil, err
	}

	return tools, func() { _ = mcpClient.Close() }, nil
}

func (s *Service) markFailed(ctx context.Context, params *AnalysisParams, summary string, logger *slog.Logger) {
	summaryPtr := &summary
	entry := &store.ReportEntry{
		ReportID:  params.ReportID,
		AlertID:   params.AlertID,
		Status:    "failed",
		Summary:   summaryPtr,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if params.Scope != nil {
		entry.EnvironmentUID = params.Scope.EnvironmentUID
		entry.ProjectUID = params.Scope.ProjectUID
	}
	if err := s.store.UpsertReport(ctx, entry); err != nil {
		logger.Error("failed to mark report as failed", "error", err)
	}
}

var observabilityToolNames = map[string]bool{
	"query_component_logs":   true,
	"query_resource_metrics": true,
	"query_traces":           true,
	"query_trace_spans":      true,
}

func classifyTools(tools []agent.Tool) (obs []prompts.ToolInfo, oc []prompts.ToolInfo) {
	for _, t := range tools {
		info := prompts.ToolInfo{Name: t.Name}
		if observabilityToolNames[t.Name] {
			obs = append(obs, info)
		} else {
			oc = append(oc, info)
		}
	}
	return obs, oc
}

func toPromptAlert(a *AlertParams) *prompts.AlertData {
	data := &prompts.AlertData{
		ID:        a.ID,
		Value:     a.Value,
		Timestamp: a.Timestamp,
		Rule: prompts.AlertRuleData{
			Name:        a.Rule.Name,
			Description: a.Rule.Description,
			Severity:    a.Rule.Severity,
		},
	}
	if a.Rule.Source != nil {
		data.Rule.Source = &prompts.AlertSourceData{
			Type:   a.Rule.Source.Type,
			Query:  a.Rule.Source.Query,
			Metric: a.Rule.Source.Metric,
		}
	}
	if a.Rule.Condition != nil {
		data.Rule.Condition = &prompts.AlertConditionData{
			Window:    a.Rule.Condition.Window,
			Interval:  a.Rule.Condition.Interval,
			Operator:  a.Rule.Condition.Operator,
			Threshold: a.Rule.Condition.Threshold,
		}
	}
	return data
}

func extractSummary(report json.RawMessage) *string {
	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(report, &parsed); err == nil && parsed.Summary != "" {
		return &parsed.Summary
	}
	return nil
}
