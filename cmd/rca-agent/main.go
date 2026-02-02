// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/openchoreo/openchoreo/internal/rca/auth"
	"github.com/openchoreo/openchoreo/internal/rca/config"
	"github.com/openchoreo/openchoreo/internal/rca/handler"
	"github.com/openchoreo/openchoreo/internal/rca/opensearch"
	"github.com/openchoreo/openchoreo/internal/rca/service"
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

	// Initialize report store (OpenSearch)
	reportStore, err := opensearch.NewReportStore(cfg, logger)
	if err != nil {
		logger.Error("Failed to create report store", "error", err)
		os.Exit(1)
	}

	// Check OpenSearch connection
	ctx := context.Background()
	if err := reportStore.CheckConnection(ctx); err != nil {
		logger.Warn("OpenSearch connection check failed", "error", err)
		// Continue anyway - connection may recover
	} else {
		logger.Info("OpenSearch connection established")
	}

	// Initialize services
	analysisService := service.NewAnalysisService(cfg, reportStore, logger)
	chatService := service.NewChatService(cfg, reportStore, logger)

	// Initialize authorization client
	authzClient := auth.NewAuthzClient(cfg, logger)

	// Initialize handlers
	analyzeHandler := handler.NewAnalyzeHandler(analysisService, logger)
	chatHandler := handler.NewChatHandler(chatService, authzClient, logger)

	// Initialize JWT middleware
	jwtMiddleware := auth.NewJWTMiddleware(cfg, logger)

	// Initialize HTTP server
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /health", healthHandler())

	// API endpoints
	mux.Handle("POST /analyze", analyzeHandler)
	// Chat endpoint requires JWT authentication
	mux.Handle("POST /chat", jwtMiddleware(chatHandler))

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.ServerPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting RCA agent server", "address", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown using signal context
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait for interrupt signal
	<-signalCtx.Done()

	logger.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server shutdown complete")
}

// healthHandler returns a handler for the health check endpoint.
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level

	// Normalize to lowercase for comparison
	levelLower := strings.ToLower(level)

	switch levelLower {
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

	// Use JSON handler for production, text handler for debug
	var handler slog.Handler
	if levelLower == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// createBootstrapLogger creates a minimal logger for early initialization.
func createBootstrapLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Use JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return slog.New(handler)
}
