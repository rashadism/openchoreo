// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"slices"

	"charm.land/fantasy"
)

// AllowedTools defines the whitelist of tools per MCP server.
// These match exactly what the Python RCA agent uses.
var AllowedTools = map[string][]string{
	"observability": {
		"get_traces",
		"get_component_logs",
		"get_project_logs",
		"get_component_resource_metrics",
	},
	"openchoreo": {
		"list_environments",
		"list_namespaces",
		"list_projects",
		"list_components",
	},
}

// FilterTools filters a list of MCP tools based on the allowed tools whitelist.
func FilterTools(tools []*Tool) []fantasy.AgentTool {
	filtered := make([]fantasy.AgentTool, 0, len(tools))

	for _, t := range tools {
		allowedForServer, serverExists := AllowedTools[t.ServerName()]
		if !serverExists {
			continue
		}

		if slices.Contains(allowedForServer, t.Name()) {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

// GetAllAllowedToolNames returns a flat list of all allowed tool names.
func GetAllAllowedToolNames() []string {
	var names []string
	for _, tools := range AllowedTools {
		names = append(names, tools...)
	}
	return names
}
