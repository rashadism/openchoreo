// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

// Transformer transforms raw MCP tool output into LLM-friendly format.
type Transformer interface {
	// Transform takes raw tool output and returns formatted string.
	Transform(content map[string]any) (string, error)
}

// transformerRegistry holds registered transformers by tool name.
var transformerRegistry = make(map[string]Transformer)

// RegisterTransformer registers a transformer for a specific tool.
func RegisterTransformer(toolName string, t Transformer) {
	transformerRegistry[toolName] = t
}

// GetTransformer returns the transformer for a tool, or nil if none exists.
func GetTransformer(toolName string) Transformer {
	return transformerRegistry[toolName]
}
