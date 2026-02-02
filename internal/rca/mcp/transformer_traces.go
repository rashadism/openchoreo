// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// TracesTransformer transforms distributed traces into hierarchical markdown.
type TracesTransformer struct{}

type spanInfo struct {
	SpanID                 string
	ParentSpanID           string
	Name                   string
	StartTime              string
	EndTime                string
	DurationNanoseconds    int64
	OpenChoreoComponentUID string
	OpenChoreoProjectUID   string
	Depth                  int
}

func (t *TracesTransformer) Transform(content map[string]any) (string, error) {
	traces, ok := content["traces"].([]any)
	if !ok || len(traces) == 0 {
		return "No traces found", nil
	}

	tookMs, _ := content["tookMs"].(float64)

	var sb strings.Builder

	sb.WriteString("## Distributed Traces\n\n")
	sb.WriteString(fmt.Sprintf("**Query Time:** %.0f ms\n", tookMs))
	sb.WriteString(fmt.Sprintf("**Total Traces:** %d\n\n", len(traces)))

	for _, traceEntry := range traces {
		trace, ok := traceEntry.(map[string]any)
		if !ok {
			continue
		}

		traceID, _ := trace["traceId"].(string)
		spans, ok := trace["spans"].([]any)
		if !ok || len(spans) == 0 {
			continue
		}

		// Parse spans
		parsedSpans := parseSpans(spans)
		if len(parsedSpans) == 0 {
			continue
		}

		// Build span tree
		spanTree := buildSpanTree(parsedSpans)

		// Calculate total duration from root spans
		totalDurationNs := int64(0)
		for _, span := range spanTree {
			if span.Depth == 0 {
				totalDurationNs += span.DurationNanoseconds
			}
		}
		totalDurationMs := float64(totalDurationNs) / 1000000

		sb.WriteString(fmt.Sprintf("### Trace ID: %s\n\n", traceID))
		sb.WriteString(fmt.Sprintf("**Total Spans:** %d\n", len(spans)))
		sb.WriteString(fmt.Sprintf("**Total Duration:** %.2f ms\n\n", totalDurationMs))

		for _, span := range spanTree {
			// Create header with depth-based markdown heading level
			headerLevel := strings.Repeat("#", span.Depth+4) // Start at #### for depth 0
			if span.Depth > 3 {
				// For very deep spans, use indentation instead
				headerLevel = "####"
				sb.WriteString(strings.Repeat("  ", span.Depth-3))
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", headerLevel, span.Name))

			durationMs := float64(span.DurationNanoseconds) / 1000000
			sb.WriteString(fmt.Sprintf("- **Duration:** %.2f ms (%d ns)\n", durationMs, span.DurationNanoseconds))

			spanIDLine := fmt.Sprintf("- **Span ID:** %s", span.SpanID)
			if span.ParentSpanID != "" {
				spanIDLine += fmt.Sprintf(" | **Parent Span ID:** %s", span.ParentSpanID)
			}
			sb.WriteString(spanIDLine + "\n")

			sb.WriteString(fmt.Sprintf("- **Time:** %s → %s\n", span.StartTime, span.EndTime))
			sb.WriteString(fmt.Sprintf("- **Component UID:** %s\n", span.OpenChoreoComponentUID))
			sb.WriteString(fmt.Sprintf("- **Project UID:** %s\n\n", span.OpenChoreoProjectUID))
		}
	}

	return sb.String(), nil
}

func parseSpans(spans []any) []*spanInfo {
	var result []*spanInfo

	for _, spanEntry := range spans {
		span, ok := spanEntry.(map[string]any)
		if !ok {
			continue
		}

		info := &spanInfo{
			SpanID:       getString(span, "spanId"),
			ParentSpanID: getString(span, "parentSpanId"),
			Name:         getString(span, "name"),
			StartTime:    getString(span, "startTime"),
			EndTime:      getString(span, "endTime"),
		}

		// Handle duration as either int or float
		if d, ok := span["durationNanoseconds"].(float64); ok {
			info.DurationNanoseconds = int64(d)
		} else if d, ok := span["durationNanoseconds"].(int64); ok {
			info.DurationNanoseconds = d
		}

		info.OpenChoreoComponentUID = getString(span, "openChoreoComponentUid")
		info.OpenChoreoProjectUID = getString(span, "openChoreoProjectUid")

		result = append(result, info)
	}

	return result
}

func buildSpanTree(spans []*spanInfo) []*spanInfo {
	if len(spans) == 0 {
		return nil
	}

	// Create a map for quick lookup
	spanMap := make(map[string]*spanInfo)
	for _, span := range spans {
		spanMap[span.SpanID] = span
	}

	// Find root spans (no parent or parent not in this trace)
	var rootSpanIDs []string
	for _, span := range spans {
		if span.ParentSpanID == "" || spanMap[span.ParentSpanID] == nil {
			rootSpanIDs = append(rootSpanIDs, span.SpanID)
		}
	}

	// Build result with depth information
	var result []*spanInfo

	var addSpanAndChildren func(spanID string, depth int)
	addSpanAndChildren = func(spanID string, depth int) {
		span, ok := spanMap[spanID]
		if !ok {
			return
		}

		span.Depth = depth
		result = append(result, span)

		// Find children
		var children []*spanInfo
		for _, s := range spans {
			if s.ParentSpanID == spanID {
				children = append(children, s)
			}
		}

		// Sort children by start time
		sort.Slice(children, func(i, j int) bool {
			return children[i].StartTime < children[j].StartTime
		})

		// Recursively add children
		for _, child := range children {
			addSpanAndChildren(child.SpanID, depth+1)
		}
	}

	// Sort root spans by start time
	var rootSpans []*spanInfo
	for _, id := range rootSpanIDs {
		if span, ok := spanMap[id]; ok {
			rootSpans = append(rootSpans, span)
		}
	}
	sort.Slice(rootSpans, func(i, j int) bool {
		return rootSpans[i].StartTime < rootSpans[j].StartTime
	})

	// Process all root spans
	for _, rootSpan := range rootSpans {
		addSpanAndChildren(rootSpan.SpanID, 0)
	}

	return result
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func init() {
	RegisterTransformer("get_traces", &TracesTransformer{})
}
