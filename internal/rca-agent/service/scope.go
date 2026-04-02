// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Scope holds the resolved UIDs for a namespace/project/environment/component.
type Scope struct {
	Namespace      string
	Project        string
	ProjectUID     string
	Environment    string
	EnvironmentUID string
	Component      string
	ComponentUID   string
}

// resolveComponentScope resolves all four resource UIDs from the OpenChoreo API.
// resolveComponentScope resolves all four resource UIDs from the OpenChoreo API.
func resolveComponentScope(ctx context.Context, apiBaseURL string, client *http.Client, namespace, project, component, environment string) (*Scope, error) {
	base := strings.TrimRight(apiBaseURL, "/") + "/api/v1"

	projectUID, err := fetchResourceUID(ctx, client, base+"/namespaces/"+namespace+"/projects/"+project)
	if err != nil {
		return nil, fmt.Errorf("resolving project UID: %w", err)
	}

	componentUID, err := fetchResourceUID(ctx, client, base+"/namespaces/"+namespace+"/components/"+component)
	if err != nil {
		return nil, fmt.Errorf("resolving component UID: %w", err)
	}

	environmentUID, err := fetchResourceUID(ctx, client, base+"/namespaces/"+namespace+"/environments/"+environment)
	if err != nil {
		return nil, fmt.Errorf("resolving environment UID: %w", err)
	}

	return &Scope{
		Namespace:      namespace,
		Project:        project,
		ProjectUID:     projectUID,
		Environment:    environment,
		EnvironmentUID: environmentUID,
		Component:      component,
		ComponentUID:   componentUID,
	}, nil
}

// resolveProjectScope resolves project and environment UIDs from the OpenChoreo API.
// resolveProjectScope resolves project and environment UIDs from the OpenChoreo API.
func resolveProjectScope(ctx context.Context, apiBaseURL string, client *http.Client, namespace, project, environment string) (*Scope, error) {
	base := strings.TrimRight(apiBaseURL, "/") + "/api/v1"

	projectUID, err := fetchResourceUID(ctx, client, base+"/namespaces/"+namespace+"/projects/"+project)
	if err != nil {
		return nil, fmt.Errorf("resolving project UID: %w", err)
	}

	environmentUID, err := fetchResourceUID(ctx, client, base+"/namespaces/"+namespace+"/environments/"+environment)
	if err != nil {
		return nil, fmt.Errorf("resolving environment UID: %w", err)
	}

	return &Scope{
		Namespace:      namespace,
		Project:        project,
		ProjectUID:     projectUID,
		Environment:    environment,
		EnvironmentUID: environmentUID,
	}, nil
}

// fetchResourceUID GETs a resource from the OpenChoreo API and extracts metadata.uid.
func fetchResourceUID(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Use-OpenAPI", "true")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d for %s", resp.StatusCode, url)
	}

	var body struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if body.Metadata.UID == "" {
		return "", fmt.Errorf("empty UID in response from %s", url)
	}

	return body.Metadata.UID, nil
}
