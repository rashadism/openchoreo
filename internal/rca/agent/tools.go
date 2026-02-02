// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/schema"
)

// TodosToolName is the name of the todos tool.
const TodosToolName = "todos"

// StructuredOutputToolName is the name of the structured output tool.
const StructuredOutputToolName = "structured_output"

//go:embed descriptions/todos.md
var todosDescription string

//go:embed descriptions/structured_output.md
var structuredOutputDescription string

// TodosParams is the parameters for the todos tool.
type TodosParams struct {
	Todos []Todo `json:"todos" description:"The updated todo list"`
}

// Todo represents a single todo item.
type Todo struct {
	Content string `json:"content" description:"What needs to be done"`
	Status  string `json:"status" description:"Task status: pending, in_progress, or completed"`
}

// NewTodosTool creates a new todos tool for task tracking.
func NewTodosTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TodosToolName,
		todosDescription,
		func(ctx context.Context, params TodosParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			// Validate statuses
			for _, todo := range params.Todos {
				switch todo.Status {
				case "pending", "in_progress", "completed":
				default:
					return fantasy.ToolResponse{}, fmt.Errorf("invalid status %q for todo %q", todo.Status, todo.Content)
				}
			}

			// Count by status
			var pending, inProgress, completed int
			for _, todo := range params.Todos {
				switch todo.Status {
				case "pending":
					pending++
				case "in_progress":
					inProgress++
				case "completed":
					completed++
				}
			}

			// Build response with current todo state
			var sb strings.Builder
			sb.WriteString("Todo list updated.\n\n")
			sb.WriteString("Current todos:\n")
			for _, todo := range params.Todos {
				var icon string
				switch todo.Status {
				case "pending":
					icon = "[ ]"
				case "in_progress":
					icon = "[~]"
				case "completed":
					icon = "[x]"
				}
				sb.WriteString(fmt.Sprintf("  %s %s\n", icon, todo.Content))
			}
			sb.WriteString(fmt.Sprintf("\nStatus: %d pending, %d in progress, %d completed",
				pending, inProgress, completed))

			return fantasy.NewTextResponse(sb.String()), nil
		})
}

// NewStructuredOutputTool creates a structured output tool with a dynamic schema.
func NewStructuredOutputTool(outputSchema any) fantasy.AgentTool {
	s := schema.Generate(reflect.TypeOf(outputSchema))
	return &structuredOutputTool{schema: s}
}

// structuredOutputTool implements fantasy.AgentTool with a dynamic schema.
type structuredOutputTool struct {
	schema          fantasy.Schema
	providerOptions fantasy.ProviderOptions
}

func (t *structuredOutputTool) Info() fantasy.ToolInfo {
	required := t.schema.Required
	if required == nil {
		required = []string{}
	}
	return fantasy.ToolInfo{
		Name:        StructuredOutputToolName,
		Description: structuredOutputDescription,
		Parameters:  schema.ToParameters(t.schema),
		Required:    required,
	}
}

func (t *structuredOutputTool) Run(ctx context.Context, params fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("Response submitted"), nil
}

func (t *structuredOutputTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *structuredOutputTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}
