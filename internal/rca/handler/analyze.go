// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package handler implements HTTP handlers for the RCA agent API.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/rca/models"
	"github.com/openchoreo/openchoreo/internal/rca/service"
)

// AnalyzeHandler handles POST /analyze requests.
type AnalyzeHandler struct {
	service *service.AnalysisService
	logger  *slog.Logger
}

// NewAnalyzeHandler creates a new analyze handler.
func NewAnalyzeHandler(svc *service.AnalysisService, logger *slog.Logger) *AnalyzeHandler {
	return &AnalyzeHandler{
		service: svc,
		logger:  logger,
	}
}

// ServeHTTP handles the analyze request.
func (h *AnalyzeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode analyze request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.ComponentUID == "" {
		writeJSONError(w, http.StatusBadRequest, "componentUid is required")
		return
	}
	if req.ProjectUID == "" {
		writeJSONError(w, http.StatusBadRequest, "projectUid is required")
		return
	}
	if req.EnvironmentUID == "" {
		writeJSONError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}
	if req.Alert.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "alert.id is required")
		return
	}
	if req.Alert.Rule.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "alert.rule.name is required")
		return
	}

	h.logger.Debug("Received analyze request",
		"component_uid", req.ComponentUID,
		"project_uid", req.ProjectUID,
		"environment_uid", req.EnvironmentUID,
		"alert_id", req.Alert.ID)

	// Convert meta to map[string]any if present
	var meta map[string]any
	if req.Meta != nil {
		if m, ok := req.Meta.(map[string]any); ok {
			meta = m
		}
	}

	// Trigger analysis
	result, err := h.service.TriggerAnalysis(r.Context(), service.AnalysisRequest{
		ComponentUID:   req.ComponentUID,
		ProjectUID:     req.ProjectUID,
		EnvironmentUID: req.EnvironmentUID,
		Alert:          req.Alert,
		Meta:           meta,
	})
	if err != nil {
		h.logger.Error("Failed to trigger analysis", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create analysis task: "+err.Error())
		return
	}

	h.logger.Info("Analysis triggered",
		"report_id", result.ReportID,
		"status", result.Status)

	writeJSON(w, http.StatusAccepted, result)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
