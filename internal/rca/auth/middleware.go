// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/openchoreo/openchoreo/internal/rca/config"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

// NewJWTMiddleware creates a new JWT authentication middleware from config.
func NewJWTMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	jwtConfig := jwt.Config{
		Disabled:                     cfg.JWTDisabled,
		JWKSURL:                      cfg.JWTJWKSURL,
		JWKSRefreshInterval:          time.Duration(cfg.JWTJWKSRefreshInterval) * time.Second,
		ValidateIssuer:               cfg.JWTIssuer,
		JWKSURLTLSInsecureSkipVerify: cfg.JWKSURLTLSInsecureSkip,
		Logger:                       logger,
	}

	// Add audience validation if configured
	if cfg.JWTAudience != "" {
		jwtConfig.ValidateAudiences = []string{cfg.JWTAudience}
	}

	return jwt.Middleware(jwtConfig)
}
