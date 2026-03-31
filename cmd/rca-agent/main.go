// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"os/signal"
	"syscall"

	anyllm "github.com/mozilla-ai/any-llm-go"
	"github.com/mozilla-ai/any-llm-go/providers/openai"

	observermiddleware "github.com/openchoreo/openchoreo/internal/observer/middleware"
	"github.com/openchoreo/openchoreo/internal/rca-agent/api"
	rcaAuthz "github.com/openchoreo/openchoreo/internal/rca-agent/authz"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
	"github.com/openchoreo/openchoreo/internal/rca-agent/service"
	"github.com/openchoreo/openchoreo/internal/rca-agent/store"
	"github.com/openchoreo/openchoreo/internal/server/middleware"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

func main() {
	// Create bootstrap logger for early initialization
	bootstrapLogger := createBootstrapLogger()

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("Failed to load configuration",
			"error", err,
			"component", "rca-agent",
			"phase", "initialization",
		)
		os.Exit(1)
	}

	// Initialize logger with proper configuration
	logger := initLogger(cfg.LogLevel)
	logger.Info("Configuration loaded successfully", "log_level", cfg.LogLevel)

	// Initialize report store
	reportStore, err := store.New(
		cfg.Report.Backend,
		cfg.Report.DatabaseURI,
		logger.With("component", "report-store"),
	)
	if err != nil {
		log.Fatalf("Failed to initialize report store: %v", err)
	}
	if err := reportStore.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize report store schema: %v", err)
	}
	defer func() {
		if closeErr := reportStore.Close(); closeErr != nil {
			logger.Error("Failed to close report store", "error", closeErr)
		}
	}()

	// Initialize authz client
	authzClient, err := rcaAuthz.NewClient(&cfg.Authz, logger.With("component", "authz-client"))
	if err != nil {
		logger.Error("Failed to create authz client", "error", err)
		os.Exit(1)
	}

	// Initialize LLM provider
	llmProvider, err := openai.New(anyllm.WithAPIKey(cfg.LLM.APIKey))
	if err != nil {
		log.Fatalf("Failed to initialize LLM provider: %v", err)
	}
	logger.Info("LLM provider initialized", "model", cfg.LLM.ModelName)

	// Initialize agent service
	agentService := service.New(llmProvider, cfg, reportStore, logger.With("component", "agent-service"))

	// Set up HTTP handler
	mux := http.NewServeMux()
	handler := api.NewHandler(logger.With("component", "api"), reportStore, authzClient, agentService)

	// Initialize JWT middleware
	jwtAuth := initJWTMiddleware(cfg, logger)

	// Build routes with middleware
	routes := middleware.NewRouteBuilder(mux)

	// Public routes (no authentication)
	routes.HandleFunc("GET /health", handler.Health)
	routes.HandleFunc("POST /api/v1alpha1/rca-agent/analyze", handler.Analyze)

	// Protected routes (JWT authentication required)
	protected := routes.With(jwtAuth)
	protected.HandleFunc("POST /api/v1alpha1/rca-agent/chat", handler.Chat)
	protected.HandleFunc("GET /api/v1/rca-agent/reports", handler.ListReports)
	protected.HandleFunc("GET /api/v1/rca-agent/reports/{report_id}", handler.GetReport)
	protected.HandleFunc("PUT /api/v1/rca-agent/reports/{report_id}", handler.UpdateReport)

	// Create HTTP server with CORS
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      observermiddleware.CORS(cfg.CORS.AllowedOrigins)(mux),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server
	go func() {
		logger.Info("Starting server", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown using signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait for interrupt signal
	<-ctx.Done()

	logger.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server shutdown complete")
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level

	level = strings.ToLower(level)
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if level == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func createBootstrapLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}

// initJWTMiddleware initializes the JWT authentication middleware from configuration.
func initJWTMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	var jwtAudiences []string
	if cfg.Auth.JWTAudience != "" {
		jwtAudiences = []string{cfg.Auth.JWTAudience}
	}

	// Create subject type resolver from configuration
	var detector *jwt.Resolver
	if len(cfg.Auth.SubjectTypes) > 0 {
		var err error
		detector, err = jwt.NewResolver(cfg.Auth.SubjectTypes)
		if err != nil {
			logger.Error("Failed to create JWT subject resolver", "error", err)
		} else {
			logger.Info("JWT subject resolver initialized", "subject_types_count", len(cfg.Auth.SubjectTypes))
		}
	}

	jwtConfig := jwt.Config{
		Disabled:                     cfg.Auth.JWTDisabled,
		JWKSURL:                      cfg.Auth.JWTJWKSURL,
		ValidateIssuer:               cfg.Auth.JWTIssuer,
		ValidateAudiences:            jwtAudiences,
		JWKSURLTLSInsecureSkipVerify: cfg.Auth.TLSInsecureSkipVerify,
		Detector:                     detector,
		Logger:                       logger,
	}

	return jwt.Middleware(jwtConfig)
}
