// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/rca/auth"
	"github.com/openchoreo/openchoreo/internal/rca/models"
	"github.com/openchoreo/openchoreo/internal/rca/service"
)

// ChatHandler handles POST /chat requests.
type ChatHandler struct {
	service     *service.ChatService
	authzClient *auth.AuthzClient
	logger      *slog.Logger
}

// NewChatHandler creates a new chat handler.
func NewChatHandler(svc *service.ChatService, authzClient *auth.AuthzClient, logger *slog.Logger) *ChatHandler {
	return &ChatHandler{
		service:     svc,
		authzClient: authzClient,
		logger:      logger,
	}
}

// ServeHTTP handles the chat request with NDJSON streaming.
func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode chat request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if req.ReportID == "" {
		writeJSONError(w, http.StatusBadRequest, "reportId is required")
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
	if len(req.Messages) == 0 {
		writeJSONError(w, http.StatusBadRequest, "messages is required")
		return
	}

	h.logger.Debug("Received chat request",
		"report_id", req.ReportID,
		"project_uid", req.ProjectUID,
		"message_count", len(req.Messages))

	// Check authorization
	componentUID := ""
	if req.ComponentUID != nil {
		componentUID = *req.ComponentUID
	}

	if h.authzClient != nil {
		if err := h.authzClient.CheckChatAuthorization(r.Context(), req.ProjectUID, componentUID); err != nil {
			h.logger.Warn("Authorization failed", "error", err)
			writeJSONError(w, http.StatusForbidden, "Access denied")
			return
		}
	}

	// Convert messages
	messages := make([]service.ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = service.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Start streaming
	events, err := h.service.StreamChat(r.Context(), service.ChatRequest{
		ReportID:       req.ReportID,
		Version:        req.Version,
		ProjectUID:     req.ProjectUID,
		EnvironmentUID: req.EnvironmentUID,
		ComponentUID:   componentUID,
		Messages:       messages,
	})
	if err != nil {
		h.logger.Error("Failed to start chat stream", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to start chat: "+err.Error())
		return
	}

	// Set streaming headers
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("Streaming not supported")
		return
	}

	encoder := json.NewEncoder(w)

	for event := range events {
		if err := encoder.Encode(event); err != nil {
			h.logger.Error("Failed to encode event", "error", err)
			return
		}
		flusher.Flush()
	}
}
