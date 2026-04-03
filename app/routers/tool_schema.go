package routers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ToolSchemaRouter registers the tool schema endpoint
func ToolSchemaRouter(r *gin.Engine) {
	r.GET("/tool-schema", toolSchemaHandler)
}

// toolSchemaHandler godoc
// @Summary Tool schema for AI agents
// @Description Returns available tools schema for AI agent integration
// @Tags Tools
// @Produce json
// @Success 200 {object} map[string]interface{} "Tool schema definition"
// @Router /tool-schema [get]
func toolSchemaHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"tools": []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "web_search",
					"description": "Search the web and return relevant results with optional content extraction.",
					"parameters": map[string]interface{}{
						"type":     "object",
						"required": []string{"query"},
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "The search query",
							},
							"search_depth": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"basic", "advanced"},
								"default":     "basic",
								"description": "basic = snippets only, advanced = full content extraction",
							},
							"topic": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"general", "news"},
								"default": "general",
							},
							"max_results": map[string]interface{}{
								"type":    "integer",
								"minimum": 1,
								"maximum": 20,
								"default": 5,
							},
							"include_answer": map[string]interface{}{
								"type":        "boolean",
								"default":     false,
								"description": "Generate an AI answer from search results (requires LLM config)",
							},
							"include_raw_content": map[string]interface{}{
								"type":        "boolean",
								"default":     false,
								"description": "Include full extracted page content",
							},
							"include_images": map[string]interface{}{
								"type":        "boolean",
								"default":     false,
								"description": "Include image search results",
							},
							"include_domains": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "string"},
								"description": "Only include results from these domains",
							},
							"exclude_domains": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "string"},
								"description": "Exclude results from these domains",
							},
							"time_range": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"day", "week", "month", "year"},
								"description": "Filter by time range",
							},
						},
					},
				},
			},
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "web_extract",
					"description": "Extract clean content from one or more URLs.",
					"parameters": map[string]interface{}{
						"type":     "object",
						"required": []string{"urls"},
						"properties": map[string]interface{}{
							"urls": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "string"},
								"minItems":    1,
								"maxItems":    20,
								"description": "URLs to extract content from",
							},
							"format": map[string]interface{}{
								"type":    "string",
								"enum":    []string{"markdown", "text"},
								"default": "markdown",
							},
						},
					},
				},
			},
		},
	})
}
