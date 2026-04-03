package routers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/TW-Fusion/fusion-search/app/middleware"
	"github.com/TW-Fusion/fusion-search/app/models"
	"github.com/TW-Fusion/fusion-search/app/services"
	"github.com/gin-gonic/gin"
)

// SearchStreamRouter sets up search stream routes with SSE
func SearchStreamRouter(
	r *gin.Engine,
	cfg *config.AppConfig,
	searchBackend services.SearchBackend,
	extractor *services.ContentExtractor,
	llmService *services.LLMService,
	rateLimiter *middleware.RateLimiter,
) {
	router := r.Group("/search/stream")
	router.Use(rateLimiter.Limit(cfg.RateLimit.SearchRate))
	router.POST("", SearchStreamHandler(cfg, searchBackend, extractor, llmService))
}

// SearchStreamHandler godoc
// @Summary Search the web with Server-Sent Events (SSE) streaming
// @Description Perform a web search and stream results back using Server-Sent Events. Supports real-time streaming of search results, extractions, and AI-generated answers.
// @Tags Search
// @Accept json
// @Produce json
// @Param request body models.SearchRequest true "Search request parameters"
// @Success 200 {string} string "SSE stream with search results and optional AI answer"
// @Failure 400 {object} map[string]string "Invalid request parameters"
// @Router /search/stream [post]
func SearchStreamHandler(
	cfg *config.AppConfig,
	searchBackend services.SearchBackend,
	extractor *services.ContentExtractor,
	llmService *services.LLMService,
) gin.HandlerFunc {
	logger := logging.GetLogger()

	return func(c *gin.Context) {
		var req models.SearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Set defaults
		if req.MaxResults == 0 {
			req.MaxResults = 5
		}
		if req.SearchDepth == "" {
			req.SearchDepth = models.SearchDepthBasic
		}
		if req.Topic == "" {
			req.Topic = models.TopicGeneral
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(cfg.Resilience.RequestTimeout)*time.Second)
		defer cancel()

		// Set SSE headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")

		start := time.Now()

		// Perform search
		opts := services.SearchOptions{
			MaxResults:     req.MaxResults,
			Topic:          string(req.Topic),
			TimeRange:      (*string)(req.TimeRange),
			IncludeDomains: req.IncludeDomains,
			ExcludeDomains: req.ExcludeDomains,
			IncludeImages:  req.IncludeImages,
		}

		backendResp, err := searchBackend.Search(ctx, req.Query, opts)
		if err != nil {
			logger.Errorw("search_backend_failed", "error", err)
			sendEvent(c, "error", map[string]interface{}{"error": err.Error()})
			return
		}

		// Stream each result
		results := make([]models.SearchResult, len(backendResp.Results))
		for i, raw := range backendResp.Results {
			result := models.SearchResult{
				Title:   raw.Title,
				URL:     raw.URL,
				Content: raw.Snippet,
				Score:   raw.Score,
			}
			results[i] = result
			sendEvent(c, "result", result)
		}

		// Stream images
		for _, img := range backendResp.Images {
			sendEvent(c, "image", map[string]interface{}{
				"url":         img.URL,
				"description": img.Description,
			})
		}

		// If advanced, stream extractions as they complete
		if req.SearchDepth == models.SearchDepthAdvanced || req.IncludeRawContent {
			for _, result := range results {
				extraction := extractor.ExtractURL(ctx, result.URL, "markdown")
				if extraction.Success() {
					sendEvent(c, "extraction", map[string]interface{}{
						"url":         result.URL,
						"raw_content": *extraction.Content,
					})
				}
			}
		}

		// AI answer generation (streamed)
		if req.IncludeAnswer && llmService != nil {
			ch, errCh := llmService.GenerateAnswerStream(ctx, req.Query, results)
			for {
				select {
				case chunk, ok := <-ch:
					if !ok {
						ch = nil
						continue
					}
					sendEvent(c, "answer_chunk", map[string]interface{}{"text": chunk})
				case err := <-errCh:
					if err != nil {
						logger.Errorw("llm_stream_failed", "error", err)
					}
					errCh = nil
				}
				if ch == nil && errCh == nil {
					break
				}
			}
			sendEvent(c, "answer_done", map[string]interface{}{})
		}

		elapsed := time.Since(start).Seconds()
		sendEvent(c, "done", map[string]interface{}{
			"response_time": elapsed,
		})
	}
}

func sendEvent(c *gin.Context, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, string(jsonData))
	c.Writer.Flush()
}
