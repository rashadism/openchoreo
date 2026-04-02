// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/agent"
)

// ToolCallRecord captures timing and metadata for a single tool call.
type ToolCallRecord struct {
	Name    string `json:"name"`
	Args    string `json:"args"`
	Elapsed float64 `json:"elapsed"`
}

// Logging tracks model and tool call timing and produces a summary.
type Logging struct {
	logger         *slog.Logger
	mu             sync.Mutex
	modelCallCount int
	toolCallCount  int
	toolCalls      []ToolCallRecord
}

func NewLogging(logger *slog.Logger) *Logging {
	return &Logging{logger: logger}
}

func (m *Logging) Name() string { return "logging" }

// WrapModelCall logs model call timing and tool calls in the response.
func (m *Logging) WrapModelCall(ctx context.Context, req *agent.ModelRequest, next agent.ModelCallHandler) (*agent.ModelResponse, error) {
	m.mu.Lock()
	m.modelCallCount++
	callNum := m.modelCallCount
	m.mu.Unlock()

	m.logger.Debug("starting model call", "call_num", callNum)

	start := time.Now()
	resp, err := next(ctx, req)
	elapsed := time.Since(start).Seconds()

	if err != nil {
		m.logger.Error("model call failed", "call_num", callNum, "elapsed_s", elapsed, "error", err)
		return nil, err
	}

	m.logger.Info("model call completed", "call_num", callNum, "elapsed_s", elapsed)

	if len(resp.Message.ToolCalls) > 0 {
		for _, tc := range resp.Message.ToolCalls {
			m.logger.Debug("tool call requested",
				"tool", tc.Function.Name,
				"args", tc.Function.Arguments,
			)
		}
	}

	return resp, nil
}

// WrapToolCall logs tool execution timing and result size.
func (m *Logging) WrapToolCall(ctx context.Context, req *agent.ToolCallRequest, next agent.ToolCallHandler) (*agent.ToolCallResponse, error) {
	toolName := req.ToolCall.Function.Name
	toolArgs := req.ToolCall.Function.Arguments

	start := time.Now()
	resp, err := next(ctx, req)
	elapsed := time.Since(start).Seconds()

	m.mu.Lock()
	m.toolCallCount++
	callNum := m.toolCallCount
	m.toolCalls = append(m.toolCalls, ToolCallRecord{
		Name:    toolName,
		Args:    toolArgs,
		Elapsed: float64(int(elapsed*100)) / 100, // round to 2 decimals
	})
	m.mu.Unlock()

	contentLen := 0
	if resp != nil {
		contentLen = len(resp.Content)
	}

	m.logger.Info("tool executed",
		"tool", toolName,
		"call_num", callNum,
		"elapsed_s", elapsed,
		"result_chars", contentLen,
	)
	m.logger.Debug("tool args", "tool", toolName, "args", toolArgs)

	return resp, err
}

// ToolCallSummary returns a JSON string of all tool call records, or empty string if none.
func (m *Logging) ToolCallSummary() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.toolCalls) == 0 {
		return ""
	}

	data, err := json.Marshal(m.toolCalls)
	if err != nil {
		return ""
	}
	return string(data)
}
