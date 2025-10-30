// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/k8s"
	"github.com/openchoreo/openchoreo/internal/observer/opensearch"
)

// OpenSearchClient interface for testing
type OpenSearchClient interface {
	Search(ctx context.Context, indices []string, query map[string]interface{}) (*opensearch.SearchResponse, error)
	GetIndexMapping(ctx context.Context, index string) (*opensearch.MappingResponse, error)
	HealthCheck(ctx context.Context) error
}

// LoggingService provides logging functionality
type LoggingService struct {
	osClient     OpenSearchClient
	queryBuilder *opensearch.QueryBuilder
	config       *config.Config
	k8sClient    *k8s.Client
	logger       *slog.Logger
}

// LogResponse represents the response structure for log queries
type LogResponse struct {
	Logs       []opensearch.LogEntry `json:"logs"`
	TotalCount int                   `json:"totalCount"`
	Took       int                   `json:"tookMs"`
}

// NewLoggingService creates a new logging service instance
func NewLoggingService(osClient OpenSearchClient, cfg *config.Config, k8sClient *k8s.Client, logger *slog.Logger) *LoggingService {
	return &LoggingService{
		osClient:     osClient,
		queryBuilder: opensearch.NewQueryBuilder(cfg.OpenSearch.IndexPrefix),
		config:       cfg,
		k8sClient:    k8sClient,
		logger:       logger,
	}
}

// GetComponentLogs retrieves logs for a specific component using V2 wildcard search
func (s *LoggingService) GetComponentLogs(ctx context.Context, params opensearch.ComponentQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting component logs",
		"component_id", params.ComponentID,
		"environment_id", params.EnvironmentID,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildComponentLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute component logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Component logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetProjectLogs retrieves logs for a specific project using V2 wildcard search
func (s *LoggingService) GetProjectLogs(ctx context.Context, params opensearch.QueryParams, componentIDs []string) (*LogResponse, error) {
	s.logger.Info("Getting project logs",
		"project_id", params.ProjectID,
		"environment_id", params.EnvironmentID,
		"component_ids", componentIDs,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildProjectLogsQuery(params, componentIDs)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute project logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Project logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetGatewayLogs retrieves gateway logs using V2 wildcard search
func (s *LoggingService) GetGatewayLogs(ctx context.Context, params opensearch.GatewayQueryParams) (*LogResponse, error) {
	s.logger.Info("Getting gateway logs",
		"organization_id", params.OrganizationID,
		"gateway_vhosts", params.GatewayVHosts,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build query with wildcard search
	query := s.queryBuilder.BuildGatewayLogsQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute gateway logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Gateway logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// GetOrganizationLogs retrieves logs for an organization with custom filters
func (s *LoggingService) GetOrganizationLogs(ctx context.Context, params opensearch.QueryParams, podLabels map[string]string) (*LogResponse, error) {
	s.logger.Info("Getting organization logs",
		"organization_id", params.OrganizationID,
		"environment_id", params.EnvironmentID,
		"pod_labels", podLabels,
		"search_phrase", params.SearchPhrase)

	// Generate indices based on time range
	indices, err := s.queryBuilder.GenerateIndices(params.StartTime, params.EndTime)
	if err != nil {
		s.logger.Error("Failed to generate indices", "error", err)
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Build organization-specific query
	query := s.queryBuilder.BuildOrganizationLogsQuery(params, podLabels)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		s.logger.Error("Failed to execute organization logs search", "error", err)
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Parse log entries
	logs := make([]opensearch.LogEntry, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		entry := opensearch.ParseLogEntry(hit)
		logs = append(logs, entry)
	}

	s.logger.Info("Organization logs retrieved",
		"count", len(logs),
		"total", response.Hits.Total.Value)

	return &LogResponse{
		Logs:       logs,
		TotalCount: response.Hits.Total.Value,
		Took:       response.Took,
	}, nil
}

// HealthCheck performs a health check on the service
func (s *LoggingService) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.osClient.HealthCheck(ctx); err != nil {
		s.logger.Error("Health check failed", "error", err)
		return fmt.Errorf("opensearch health check failed: %w", err)
	}

	s.logger.Debug("Health check passed")
	return nil
}

// RCARequest represents a request to perform root cause analysis
type RCARequest struct {
	ProjectID   string          `json:"project_id"`
	ComponentID string          `json:"component_id"`
	Environment string          `json:"environment"`
	Timestamp   string          `json:"timestamp"`
	Context     json.RawMessage `json:"context"` // Arbitrary JSON object
}

// RCAResponse represents the response from creating an RCA job
type RCAResponse struct {
	JobName      string            `json:"jobName"`
	JobNamespace string            `json:"jobNamespace"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	Labels       map[string]string `json:"labels"`
}

// KickoffRCA creates a Kubernetes job to perform root cause analysis
func (s *LoggingService) KickoffRCA(ctx context.Context, req RCARequest) (*RCAResponse, error) {
	// Check if RCA is enabled
	if !s.config.RCA.Enabled {
		return nil, fmt.Errorf("RCA feature is not enabled")
	}

	// Check if k8s client is initialized
	if s.k8sClient == nil {
		s.logger.Error("Kubernetes client is not initialized")
		return nil, fmt.Errorf("kubernetes client not available")
	}

	s.logger.Info("Creating RCA job",
		"project_id", req.ProjectID,
		"component_id", req.ComponentID,
		"environment", req.Environment)

	// TODO: Change this
	jobName := fmt.Sprintf("rca-%s-%x", req.ProjectID, rand.Intn(0xfffff))

	// Build job spec
	jobSpec := k8s.JobSpec{
		Name:                    jobName,
		Namespace:               s.config.RCA.Namespace,
		ProjectID:               req.ProjectID,
		ComponentID:             req.ComponentID,
		Environment:             req.Environment,
		Timestamp:               req.Timestamp,
		ContextJSON:             req.Context,
		ImageRepository:         s.config.RCA.ImageRepository,
		ImageTag:                s.config.RCA.ImageTag,
		ImagePullPolicy:         s.config.RCA.ImagePullPolicy,
		TTLSecondsAfterFinished: &s.config.RCA.TTLSecondsAfterFinished,
		ResourceLimitsCPU:       s.config.RCA.Resources.Limits.CPU,
		ResourceLimitsMemory:    s.config.RCA.Resources.Limits.Memory,
		ResourceRequestsCPU:     s.config.RCA.Resources.Requests.CPU,
		ResourceRequestsMemory:  s.config.RCA.Resources.Requests.Memory,
	}

	// Create the job (with ConfigMap for context)
	job, err := s.k8sClient.CreateJob(ctx, jobSpec)
	if err != nil {
		s.logger.Error("Failed to create RCA job", "error", err)
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	s.logger.Info("RCA job created successfully", "job_name", job.Name, "namespace", job.Namespace)

	// Build response
	response := &RCAResponse{
		JobName:      job.Name,
		JobNamespace: job.Namespace,
		Status:       "Created",
		CreatedAt:    job.CreationTimestamp.Format(time.RFC3339),
		Labels:       job.Labels,
	}

	return response, nil
}
