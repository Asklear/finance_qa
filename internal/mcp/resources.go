package mcp

import (
	"encoding/json"
	"os"
)

func (s *Server) handleResourcesList(req *Request) error {
	resources := []Resource{}

	if s.skillPath != "" {
		resources = append(resources, Resource{
			URI:         "financeqa://skill",
			Name:        "SKILL.md",
			Description: "FinanceQA skill contract and usage guide",
			MimeType:    "text/markdown",
		})
	}
	if s.appendixPath != "" {
		resources = append(resources, Resource{
			URI:         "financeqa://appendix",
			Name:        "SKILL_APPENDIX.md",
			Description: "FinanceQA skill appendix with detailed rules",
			MimeType:    "text/markdown",
		})
	}

	return s.sendResponse(req.ID, ResourcesListResult{Resources: resources})
}

func (s *Server) handleResourcesRead(req *Request) error {
	var params ResourceReadParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.sendErrorResponse(req.ID, -32602, "Invalid params", err.Error())
	}

	var content []byte
	var mimeType string

	switch params.URI {
	case "financeqa://skill":
		if s.skillPath == "" {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		var err error
		content, err = os.ReadFile(s.skillPath)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32603, "Failed to read skill", err.Error())
		}
		mimeType = "text/markdown"
	case "financeqa://appendix":
		if s.appendixPath == "" {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		var err error
		content, err = os.ReadFile(s.appendixPath)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32603, "Failed to read appendix", err.Error())
		}
		mimeType = "text/markdown"
	default:
		var err error
		content, err = os.ReadFile(params.URI)
		if err != nil {
			return s.sendErrorResponse(req.ID, -32602, "Resource not found", params.URI)
		}
		mimeType = "application/octet-stream"
	}

	result := ResourceReadResult{
		Contents: []ResourceContent{
			{
				URI:      params.URI,
				MimeType: mimeType,
				Text:     string(content),
			},
		},
	}
	return s.sendResponse(req.ID, result)
}
