// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	rcaAuthz "github.com/openchoreo/openchoreo/internal/rca-agent/authz"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
)

// Handler implements the RCA agent HTTP API.
type Handler struct {
	logger      *slog.Logger
	reportStore store.ReportStore
	authzClient authzcore.PDP
}

// NewHandler creates a new API handler.
func NewHandler(logger *slog.Logger, reportStore store.ReportStore, authzClient authzcore.PDP) *Handler {
	return &Handler{
		logger:      logger,
		reportStore: reportStore,
		authzClient: authzClient,
	}
}

// Analyze handles POST /api/v1alpha1/rca-agent/analyze.
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Namespace == "" || req.Project == "" || req.Component == "" || req.Environment == "" {
		writeError(w, http.StatusBadRequest, "namespace, project, component, and environment are required")
		return
	}

	if req.Alert.ID == "" {
		writeError(w, http.StatusBadRequest, "alert.id is required")
		return
	}

	h.logger.Info("analyze request received",
		"namespace", req.Namespace,
		"project", req.Project,
		"component", req.Component,
		"alert_id", req.Alert.ID,
	)

	// TODO: implement analysis
	writeJSON(w, http.StatusNotImplemented, ErrorResponse{Detail: "not implemented"})
}

// Chat handles POST /api/v1alpha1/rca-agent/chat.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ReportID == "" {
		writeError(w, http.StatusBadRequest, "reportId is required")
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages must not be empty")
		return
	}

	// Authorize
	resourceType, resourceID, hierarchy := rcaAuthz.ComponentScopeAuthz(req.Namespace, req.Project, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	h.logger.Info("chat request received",
		"report_id", req.ReportID,
		"namespace", req.Namespace,
		"message_count", len(req.Messages),
	)

	// TODO: implement chat streaming
	writeJSON(w, http.StatusNotImplemented, ErrorResponse{Detail: "not implemented"})
}

// ListReports handles GET /api/v1/rca-agent/reports.
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	project := q.Get("project")
	environment := q.Get("environment")
	namespace := q.Get("namespace")
	startTime := q.Get("startTime")
	endTime := q.Get("endTime")

	if project == "" || environment == "" || namespace == "" || startTime == "" || endTime == "" {
		writeError(w, http.StatusUnprocessableEntity, "project, environment, namespace, startTime, and endTime are required")
		return
	}

	// Authorize
	resourceType, resourceID, hierarchy := rcaAuthz.ComponentScopeAuthz(namespace, project, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	limit := 100
	if l := q.Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 || parsed > 10000 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 10000")
			return
		}
		limit = parsed
	}

	sort := "DESC"
	if s := q.Get("sort"); s != "" {
		switch s {
		case "asc":
			sort = "ASC"
		case "desc":
			sort = "DESC"
		default:
			writeError(w, http.StatusBadRequest, "sort must be asc or desc")
			return
		}
	}

	status := q.Get("status")
	if status != "" && status != "pending" && status != "completed" && status != "failed" {
		writeError(w, http.StatusBadRequest, "status must be pending, completed, or failed")
		return
	}

	entries, total, err := h.reportStore.ListReports(r.Context(), store.QueryParams{
		ProjectUID:     project,
		EnvironmentUID: environment,
		Namespace:      namespace,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		SortOrder:      sort,
		Status:         status,
	})
	if err != nil {
		h.logger.Error("failed to list reports", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list reports")
		return
	}

	reports := make([]RCAReportSummary, 0, len(entries))
	for _, e := range entries {
		reports = append(reports, RCAReportSummary{
			AlertID:   e.AlertID,
			ReportID:  e.ReportID,
			Timestamp: e.Timestamp,
			Summary:   e.Summary,
			Status:    e.Status,
		})
	}

	writeJSON(w, http.StatusOK, RCAReportsResponse{
		Reports:    reports,
		TotalCount: total,
	})
}

// GetReport handles GET /api/v1/rca-agent/reports/{report_id}.
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("report_id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "report_id is required")
		return
	}

	entry, err := h.reportStore.GetReport(r.Context(), reportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		h.logger.Error("failed to get report", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}

	// Authorize using the report's stored project
	resourceType, resourceID2, hierarchy := rcaAuthz.ComponentScopeAuthz("", entry.ProjectUID, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionViewRCAReport, resourceType, resourceID2, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	resp := RCAReportDetailed{
		AlertID:   entry.AlertID,
		ReportID:  entry.ReportID,
		Timestamp: entry.Timestamp,
		Status:    entry.Status,
	}

	if entry.Report != nil {
		var report RCAReport
		if err := json.Unmarshal([]byte(*entry.Report), &report); err != nil {
			h.logger.Error("failed to unmarshal report JSON", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to parse stored report")
			return
		}
		resp.Report = &report
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateReport handles PUT /api/v1/rca-agent/reports/{report_id}.
func (h *Handler) UpdateReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("report_id")
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "report_id is required")
		return
	}

	var req ReportUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if hasOverlap(req.AppliedIndices, req.DismissedIndices) {
		writeError(w, http.StatusBadRequest, "appliedIndices and dismissedIndices must not overlap")
		return
	}

	// Get existing report
	entry, err := h.reportStore.GetReport(r.Context(), reportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		h.logger.Error("failed to get report for update", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get report")
		return
	}

	// Authorize using the report's stored project
	resourceType, resourceID2, hierarchy := rcaAuthz.ComponentScopeAuthz("", entry.ProjectUID, "")
	if err := rcaAuthz.CheckAuthorization(
		r.Context(), h.logger, h.authzClient,
		rcaAuthz.ActionUpdateRCAReport, resourceType, resourceID2, hierarchy,
	); err != nil {
		handleAuthzError(w, err)
		return
	}

	if entry.Report == nil {
		writeError(w, http.StatusBadRequest, "report has no content to update")
		return
	}

	// Parse the stored report JSON, apply status changes, re-serialize
	var reportData map[string]any
	if err := json.Unmarshal([]byte(*entry.Report), &reportData); err != nil {
		h.logger.Error("failed to unmarshal report for update", "report_id", reportID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to parse stored report")
		return
	}

	// Update action statuses in the report (only valid transitions)
	changed, err := applyActionStatusUpdates(reportData, req.AppliedIndices, req.DismissedIndices)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if changed {
		updatedJSON, err := json.Marshal(reportData)
		if err != nil {
			h.logger.Error("failed to marshal updated report", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to serialize updated report")
			return
		}

		if err := h.reportStore.UpdateActionStatuses(r.Context(), reportID, string(updatedJSON)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "report not found")
				return
			}
			h.logger.Error("failed to update report", "report_id", reportID, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update report")
			return
		}
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "ok"})
}

// Health handles GET /health.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, StatusResponse{Status: "healthy"})
}

// applyActionStatusUpdates modifies the recommended_actions in the report JSON.
// Returns true if any changes were made. Only performs valid transitions:
//   - applied: revised → applied
//   - dismissed: revised|suggested → dismissed
func applyActionStatusUpdates(report map[string]any, applied, dismissed []int) (bool, error) {
	result, ok := report["result"].(map[string]any)
	if !ok {
		return false, nil
	}

	recommendations, ok := result["recommendations"].(map[string]any)
	if !ok {
		return false, nil
	}

	actions, ok := recommendations["recommended_actions"].([]any)
	if !ok {
		return false, nil
	}

	appliedSet := make(map[int]struct{}, len(applied))
	for _, idx := range applied {
		appliedSet[idx] = struct{}{}
	}

	dismissedSet := make(map[int]struct{}, len(dismissed))
	for _, idx := range dismissed {
		dismissedSet[idx] = struct{}{}
	}

	changed := false
	for i, raw := range actions {
		action, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		current, _ := action["status"].(string)

		if _, inApplied := appliedSet[i]; inApplied && current == "revised" {
			action["status"] = "applied"
			changed = true
		} else if _, inDismissed := dismissedSet[i]; inDismissed && (current == "revised" || current == "suggested") {
			action["status"] = "dismissed"
			changed = true
		}
	}

	return changed, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, ErrorResponse{Detail: detail})
}

func handleAuthzError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rcaAuthz.ErrAuthzUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, rcaAuthz.ErrAuthzForbidden):
		writeError(w, http.StatusForbidden, "insufficient permissions")
	default:
		writeError(w, http.StatusInternalServerError, "authorization check failed")
	}
}

func hasOverlap(a, b []int) bool {
	set := make(map[int]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; ok {
			return true
		}
	}
	return false
}
