// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"
)

const (
	labelComponentUID   = "openchoreo.dev/component-uid"
	labelEnvironmentUID = "openchoreo.dev/environment-uid"
	labelProjectUID     = "openchoreo.dev/project-uid"
)

// ComponentLogsTransformer transforms component logs into markdown table.
type ComponentLogsTransformer struct{}

func (t *ComponentLogsTransformer) Transform(content map[string]any) (string, error) {
	logs, ok := content["logs"].([]any)
	if !ok || len(logs) == 0 {
		return "No logs found", nil
	}

	var sb strings.Builder

	// Extract labels from first log
	firstLog, ok := logs[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid log format")
	}

	labels, _ := firstLog["labels"].(map[string]any)
	componentUID := getLabel(labels, labelComponentUID, "N/A")
	environmentUID := getLabel(labels, labelEnvironmentUID, "N/A")
	projectUID := getLabel(labels, labelProjectUID, "N/A")

	sb.WriteString("## Component Logs\n\n")
	sb.WriteString(fmt.Sprintf("**Component UID:** %s\n", componentUID))
	sb.WriteString(fmt.Sprintf("**Environment UID:** %s\n", environmentUID))
	sb.WriteString(fmt.Sprintf("**Project UID:** %s\n\n", projectUID))

	sb.WriteString("Timestamp | Log Level | Message\n")
	sb.WriteString("--- | --- | ---\n")

	for _, logEntry := range logs {
		log, ok := logEntry.(map[string]any)
		if !ok {
			continue
		}

		timestamp, _ := log["timestamp"].(string)
		logLevel, _ := log["logLevel"].(string)
		if logLevel == "" || logLevel == "UNDEFINED" {
			logLevel = "INFO"
		}
		message, _ := log["log"].(string)

		// Escape pipe characters in message for markdown table
		message = strings.ReplaceAll(message, "|", "\\|")
		message = strings.ReplaceAll(message, "\n", " ")

		sb.WriteString(fmt.Sprintf("%s | %s | %s\n", timestamp, logLevel, message))
	}

	return sb.String(), nil
}

// ProjectLogsTransformer transforms project logs grouped by component.
type ProjectLogsTransformer struct{}

func (t *ProjectLogsTransformer) Transform(content map[string]any) (string, error) {
	logs, ok := content["logs"].([]any)
	if !ok || len(logs) == 0 {
		return "No logs found", nil
	}

	// Group logs by component
	logsByComponent := make(map[string][]map[string]any)
	var projectUID, environmentUID string

	for _, logEntry := range logs {
		log, ok := logEntry.(map[string]any)
		if !ok {
			continue
		}

		labels, _ := log["labels"].(map[string]any)
		componentUID := getLabel(labels, labelComponentUID, "unknown")

		if projectUID == "" {
			projectUID = getLabel(labels, labelProjectUID, "N/A")
		}
		if environmentUID == "" {
			environmentUID = getLabel(labels, labelEnvironmentUID, "N/A")
		}

		logsByComponent[componentUID] = append(logsByComponent[componentUID], log)
	}

	var sb strings.Builder

	sb.WriteString("## Project Logs\n\n")
	sb.WriteString(fmt.Sprintf("**Project UID:** %s\n", projectUID))
	sb.WriteString(fmt.Sprintf("**Environment UID:** %s\n\n", environmentUID))

	for componentUID, componentLogs := range logsByComponent {
		sb.WriteString(fmt.Sprintf("**Component UID:** %s\n\n", componentUID))
		sb.WriteString("Timestamp | Log Level | Message\n")
		sb.WriteString("--- | --- | ---\n")

		for _, log := range componentLogs {
			timestamp, _ := log["timestamp"].(string)
			logLevel, _ := log["logLevel"].(string)
			if logLevel == "" || logLevel == "UNDEFINED" {
				logLevel = "INFO"
			}
			message, _ := log["log"].(string)

			// Escape pipe characters in message for markdown table
			message = strings.ReplaceAll(message, "|", "\\|")
			message = strings.ReplaceAll(message, "\n", " ")

			sb.WriteString(fmt.Sprintf("%s | %s | %s\n", timestamp, logLevel, message))
		}

		sb.WriteString("\n")
	}

	return sb.String(), nil
}

func getLabel(labels map[string]any, key, defaultValue string) string {
	if labels == nil {
		return defaultValue
	}
	if v, ok := labels[key].(string); ok && v != "" {
		return v
	}
	return defaultValue
}

func init() {
	RegisterTransformer("get_component_logs", &ComponentLogsTransformer{})
	RegisterTransformer("get_project_logs", &ProjectLogsTransformer{})
}
