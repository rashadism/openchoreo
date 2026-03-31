// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authRoundTripper injects a Bearer token into every request.
type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// authedHTTPClient creates an HTTP client that injects a Bearer token.
// No global Timeout is set because MCP uses long-lived SSE streams;
// per-request timeouts are controlled via context instead.
func authedHTTPClient(token string, tlsInsecure bool) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if tlsInsecure {
		base.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // intentional for self-signed certs
	}
	return &http.Client{
		Transport: &authRoundTripper{token: token, base: base},
	}
}

// fetchOAuth2Token performs an OAuth2 client_credentials token fetch.
// Matches the Python OAuth2ClientCredentialsAuth behavior: POST with
// grant_type=client_credentials, client_id, client_secret in form body.
func fetchOAuth2Token(ctx context.Context, tokenURL, clientID, clientSecret string, tlsInsecure bool) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching OAuth2 token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OAuth2 token endpoint returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("OAuth2 token response has empty access_token")
	}

	return tokenResp.AccessToken, nil
}
