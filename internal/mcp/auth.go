package mcp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// AuthScope is the authorization scope granted by a configured MCP token.
type AuthScope string

const (
	ScopeRead  AuthScope = "read"
	ScopeAdmin AuthScope = "admin"
)

// ExtractBearerToken extracts the token from an Authorization header.
func ExtractBearerToken(headerValue string) string {
	headerValue = strings.TrimSpace(headerValue)
	if !strings.HasPrefix(headerValue, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(headerValue, "Bearer "))
}

// ValidateBearerToken validates Authorization: Bearer using fixed-length hashes.
func ValidateBearerToken(headerValue, expectedToken string) bool {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return false
	}
	provided := ExtractBearerToken(headerValue)
	if provided == "" {
		return false
	}

	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expectedToken))
	return hmac.Equal(providedHash[:], expectedHash[:])
}

// LoadTokenFile reads and trims a token file, rejecting empty content.
func LoadTokenFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("token file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("token file is empty")
	}
	return token, nil
}

// RedactAuthorization returns a log-safe Authorization header value.
func RedactAuthorization(value string) string {
	if ExtractBearerToken(value) != "" {
		return "Bearer <redacted>"
	}
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "<redacted>"
}

// ScopeAllowsTool reports whether scope can call tool with args.
func ScopeAllowsTool(scope AuthScope, tool string, args map[string]any) bool {
	switch scope {
	case ScopeAdmin:
		return isFinanceTool(tool)
	case ScopeRead:
		switch tool {
		case "finance-query", "finance-host-data":
			return true
		case "finance-dimensions":
			action, _ := args["action"].(string)
			return action == "" || action == "list"
		default:
			return false
		}
	default:
		return false
	}
}

func financeToolsForScope(scope AuthScope) []Tool {
	if scope == ScopeAdmin {
		return financeTools()
	}
	if scope != ScopeRead {
		return nil
	}

	out := make([]Tool, 0, 3)
	for _, tool := range financeTools() {
		if !ScopeAllowsTool(ScopeRead, tool.Name, map[string]any{"action": "list"}) {
			continue
		}
		if tool.Name == "finance-dimensions" {
			tool = cloneTool(tool)
			if props, ok := tool.InputSchema["properties"].(map[string]any); ok {
				if action, ok := props["action"].(map[string]any); ok {
					action["enum"] = []string{"list"}
				}
			}
		}
		out = append(out, tool)
	}
	return out
}

func cloneTool(tool Tool) Tool {
	raw, err := json.Marshal(tool)
	if err != nil {
		return tool
	}
	var cloned Tool
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return tool
	}
	return cloned
}

func isFinanceTool(name string) bool {
	for _, tool := range financeTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}
