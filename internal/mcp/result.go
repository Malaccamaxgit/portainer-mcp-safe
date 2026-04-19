package mcp

import (
	"encoding/json"

	"github.com/Malaccamaxgit/portainer-mcp-safe/internal/safety"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *PortainerMCPServer) newJSONResult(payload any, note *safety.Note) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return s.newTextResult(string(data), payload, note), nil
}

func (s *PortainerMCPServer) newTextResult(text string, structuredPayload any, note *safety.Note) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: text,
			},
		},
	}

	if note != nil {
		payload, err := json.Marshal(map[string]any{
			"result":  structuredPayload,
			"_safety": note,
		})
		if err == nil {
			result.Content = append(result.Content, mcp.TextContent{
				Type: "text",
				Text: string(payload),
			})
		}
	}

	return result
}

func (s *PortainerMCPServer) newSafetyErrorResult(message string, note *safety.Note) *mcp.CallToolResult {
	result := s.newTextResult(message, map[string]any{"error": message}, note)
	result.IsError = true
	return result
}
