// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/agent"
)

// newRemedTools creates the 5 local tools for the remediation agent.
// These call the OpenChoreo REST API directly (not MCP), matching the
// Python tool factories in tool_registry.py.
func newRemedTools(apiBaseURL string, httpClient *http.Client) []agent.Tool {
	base := strings.TrimRight(apiBaseURL, "/") + "/api/v1"

	return []agent.Tool{
		{
			Name:        "list_release_bindings",
			Description: "List release bindings for a component. Returns the full binding spec including current workloadOverrides, traitEnvironmentConfigs, and componentTypeEnvironmentConfigs.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name to filter by"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				return apiGet(ctx, httpClient, base+"/namespaces/"+p.Namespace+"/releasebindings", map[string]string{"component": p.Component})
			},
		},
		{
			Name:        "get_component_workloads",
			Description: "Get workloads for a component including container specs, env vars, and endpoints.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "project", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"project":   map[string]any{"type": "string", "description": "Project name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Project   string `json:"project"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				return apiGet(ctx, httpClient, base+"/namespaces/"+p.Namespace+"/workloads", map[string]string{"project": p.Project, "component": p.Component})
			},
		},
		{
			Name:        "get_component_release_schema",
			Description: "Get the JSON Schema for a component's trait and componentType overrides. Source of truth for valid override fields.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				return apiGet(ctx, httpClient, base+"/namespaces/"+p.Namespace+"/components/"+p.Component+"/schema", nil)
			},
		},
		{
			Name:        "list_component_traits",
			Description: "List traits attached to a component with their base parameter values.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "component"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"component": map[string]any{"type": "string", "description": "Component name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Component string `json:"component"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				// Fetch full component, extract spec.traits
				raw, err := apiGet(ctx, httpClient, base+"/namespaces/"+p.Namespace+"/components/"+p.Component, nil)
				if err != nil {
					return "", err
				}
				var comp struct {
					Spec struct {
						Traits json.RawMessage `json:"traits"`
					} `json:"spec"`
				}
				if err := json.Unmarshal([]byte(raw), &comp); err != nil {
					return raw, nil
				}
				if comp.Spec.Traits == nil {
					return "[]", nil
				}
				return string(comp.Spec.Traits), nil
			},
		},
		{
			Name:        "list_components",
			Description: "List components in a project.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"namespace", "project"},
				"properties": map[string]any{
					"namespace": map[string]any{"type": "string", "description": "Namespace name"},
					"project":   map[string]any{"type": "string", "description": "Project name"},
				},
			},
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					Namespace string `json:"namespace"`
					Project   string `json:"project"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", err
				}
				return apiGet(ctx, httpClient, base+"/namespaces/"+p.Namespace+"/components", map[string]string{"project": p.Project})
			},
		},
	}
}

// apiGet calls the OpenChoreo REST API with X-Use-OpenAPI header.
func apiGet(ctx context.Context, client *http.Client, url string, params map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Use-OpenAPI", "true")

	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
