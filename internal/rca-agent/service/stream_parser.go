// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"regexp"
)

// incompleteEscape matches trailing incomplete unicode escapes or lone backslashes
// that break JSON parsing of partial content.
var incompleteEscape = regexp.MustCompile(`(\\u[0-9a-fA-F]{0,3}|\\)$`)

// chatResponseParser extracts clean message deltas from streaming structured
// JSON output, extracting clean message deltas for streaming to clients.
//
// The LLM streams JSON tokens like `{"message": "Here is the `. This parser
// accumulates chunks, tries to parse partial JSON, extracts the `message`
// field, and returns only the new text since the last successful parse.
type chatResponseParser struct {
	buffer  string
	message string
	actions []any
}

// push appends a chunk and returns the new message delta, or empty string
// if no new content could be extracted.
func (p *chatResponseParser) push(chunk string) string {
	p.buffer += chunk
	parsed := parsePartialJSON(p.buffer)
	if parsed == nil {
		return ""
	}

	if actions, ok := parsed["actions"]; ok {
		if arr, ok := actions.([]any); ok {
			p.actions = arr
		}
	}

	newMessage, _ := parsed["message"].(string)
	if len(newMessage) > len(p.message) {
		delta := newMessage[len(p.message):]
		p.message = newMessage
		return delta
	}

	return ""
}

// parsePartialJSON attempts to parse a potentially incomplete JSON string.
// On failure, cleans up trailing incomplete escapes and retries.
// Returns nil if parsing fails entirely.
func parsePartialJSON(s string) map[string]any {
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err == nil {
		return result
	}

	// Clean up incomplete unicode escapes and trailing backslashes, then retry.
	cleaned := incompleteEscape.ReplaceAllString(s, "")
	if cleaned == s {
		return nil
	}

	var result2 map[string]any
	if err := json.Unmarshal([]byte(cleaned), &result2); err == nil {
		return result2
	}

	return nil
}
