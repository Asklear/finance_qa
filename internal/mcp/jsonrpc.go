package mcp

import (
	"encoding/json"
	"fmt"

	"financeqa/internal/support"
)

func (s *Server) sendResponse(id interface{}, result any) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return s.writeResponse(resp)
}

func (s *Server) sendErrorResponse(id interface{}, code int, message, data string) error {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &ErrorObject{
			Code:    code,
			Message: support.SanitizeDSN(message),
			Data:    support.SanitizeDSN(data),
		},
	}
	return s.writeResponse(resp)
}

func (s *Server) sendToolResponse(id interface{}, toolName, operation string, result any) error {
	result = s.bridgeEnvelope(toolName, operation, result)

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return s.sendErrorResponse(id, -32603, "Failed to marshal result", err.Error())
	}

	toolResult := ToolResult{
		Content: []ContentItem{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}
	return s.sendResponse(id, toolResult)
}

func (s *Server) sendNotification(method string, params any) error {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return s.writeResponse(notif)
}

func (s *Server) writeResponse(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(s.stdout, string(data)); err != nil {
		return err
	}
	return nil
}
