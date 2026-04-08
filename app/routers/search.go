package routers

import (
	"context"
	"crypto/sha256"
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

// SearchRouter sets up search routes
func SearchRouter(
	r *gin.Engine,
	cfg *config.AppConfig,
	cache *services.CacheService,
	searchBackend services.SearchBackend,
	extractor *services.ContentExtractor,
	llmService *services.LLMService,
	reranker *services.RerankerService,
	rateLimiter *middleware.RateLimiter,
) {
	router := r.Group("/search")
	router.Use(rateLimiter.Limit(cfg.RateLimit.SearchRate))
	router.POST("", SearchHandler(cfg, cache, searchBackend, extractor, llmService, reranker))
}

// SearchHandler godoc
// @Summary Search the web and return relevant results
// @Description Perform a web search with optional content extraction, AI answer generation, and image search. Supports basic (snippets only) and advanced (full content extraction) search depths.
// @Tags Search
// @Accept json
// @Produce json
// @Param request body models.SearchRequest true "Search request parameters"
// @Success 200 {object} models.SearchResponse "Successful search response"
// @Failure 400 {object} map[string]string "Invalid request parameters"
// @Failure 503 {object} map[string]string "Search service unavailable"
// @Router /search [post]
func SearchHandler(
	cfg *config.AppConfig,
	cache *services.CacheService,
	searchBackend services.SearchBackend,
	extractor *services.ContentExtractor,
	llmService *services.LLMService,
	reranker *services.RerankerService,
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

		start := time.Now()

		// Check cache
		paramsHash := hashParams(req)
		cached, err := cache.GetSearch(req.Query, paramsHash)
		if err == nil && cached != nil {
			elapsed := time.Since(start).Seconds()
			c.JSON(http.StatusOK, models.SearchResponse{
				Query:        req.Query,
				Answer:       getString(cached, "answer"),
				Results:      getSearchResults(cached),
				Images:       getImages(cached),
				ResponseTime: elapsed,
			})
			return
		}

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
			// Try stale cache
			cached, err := cache.GetSearch(req.Query, paramsHash)
			if err == nil && cached != nil {
				elapsed := time.Since(start).Seconds()
				logger.Infow("serving_stale_cache", "query", req.Query)
				c.JSON(http.StatusOK, models.SearchResponse{
					Query:        req.Query,
					Answer:       getString(cached, "answer"),
					Results:      getSearchResults(cached),
					Images:       getImages(cached),
					ResponseTime: elapsed,
				})
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Search service unavailable"})
			return
		}

		// Build results
		results := make([]models.SearchResult, len(backendResp.Results))
		for i, raw := range backendResp.Results {
			results[i] = models.SearchResult{
				Title:   raw.Title,
				URL:     raw.URL,
				Content: raw.Snippet,
				Score:   raw.Score,
			}
		}

		// Rerank results
		if reranker != nil && cfg.Rerank.Enabled && len(results) > 0 {
			resultMaps := make([]map[string]interface{}, len(results))
			for i, result := range results {
				resultMaps[i] = map[string]interface{}{
					"title":   result.Title,
					"url":     result.URL,
					"content": result.Content,
					"score":   result.Score,
				}
			}

			reranked := reranker.Rerank(req.Query, resultMaps, req.MaxResults)
			newResults := make([]models.SearchResult, 0, len(reranked))
			for _, item := range reranked {
				sr := models.SearchResult{}
				if title, ok := item["title"].(string); ok {
					sr.Title = title
				}
				if url, ok := item["url"].(string); ok {
					sr.URL = url
				}
				if content, ok := item["content"].(string); ok {
					sr.Content = content
				}
				if score, ok := item["score"].(float64); ok {
					sr.Score = score
				}
				newResults = append(newResults, sr)
			}
			results = newResults
		}

		// Advanced depth: fetch and extract content
		if req.SearchDepth == models.SearchDepthAdvanced || req.IncludeRawContent {
			urls := make([]string, len(results))
			for i, r := range results {
				urls[i] = r.URL
			}

			extractions := extractor.ExtractURLs(ctx, urls, "markdown")
			pairs := [][2]string{}
			for i, result := range results {
				if i < len(extractions) && extractions[i].Success() {
					content := *extractions[i].Content
					results[i].RawContent = &content
					pairs = append(pairs, [2]string{result.URL, content})
				}
			}
			if len(pairs) > 0 {
				cache.SetExtractBatch(pairs)
			}
		}

		// Images
		images := make([]models.ImageResult, len(backendResp.Images))
		for i, img := range backendResp.Images {
			images[i] = models.ImageResult{
				URL:         img.URL,
				Description: img.Description,
			}
		}

		// AI answer generation
		var answer *string
		if req.IncludeAnswer && llmService != nil {
			answer, err = cache.GetAnswer(req.Query, paramsHash)
			if err != nil || answer == nil {
				answer, err = llmService.GenerateAnswer(ctx, req.Query, results)
				if err == nil && answer != nil {
					cache.SetAnswer(req.Query, paramsHash, *answer)
				}
			}
		}

		elapsed := time.Since(start).Seconds()
		response := models.SearchResponse{
			Query:        req.Query,
			Answer:       answer,
			Results:      results,
			Images:       images,
			ResponseTime: elapsed,
		}

		// Cache response
		cacheData := map[string]interface{}{
			"query":   response.Query,
			"results": response.Results,
			"images":  response.Images,
		}
		if response.Answer != nil {
			cacheData["answer"] = *response.Answer
		}
		cache.SetSearch(req.Query, paramsHash, cacheData)

		c.JSON(http.StatusOK, response)
	}
}

func hashParams(req models.SearchRequest) string {
	dataMap := map[string]interface{}{
		"search_depth":        req.SearchDepth,
		"topic":               req.Topic,
		"max_results":         req.MaxResults,
		"include_answer":      req.IncludeAnswer,
		"include_raw_content": req.IncludeRawContent,
		"include_images":      req.IncludeImages,
		"include_domains":     req.IncludeDomains,
		"exclude_domains":     req.ExcludeDomains,
		"time_range":          req.TimeRange,
	}
	data, _ := json.Marshal(dataMap)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:6])
}

func getString(m map[string]interface{}, key string) *string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return &str
		}
	}
	return nil
}

func getSearchResults(m map[string]interface{}) []models.SearchResult {
	if val, ok := m["results"]; ok {
		if results, ok := val.([]interface{}); ok {
			out := make([]models.SearchResult, 0)
			for _, r := range results {
				if rm, ok := r.(map[string]interface{}); ok {
					result := models.SearchResult{}
					if v, ok := rm["title"].(string); ok {
						result.Title = v
					}
					if v, ok := rm["url"].(string); ok {
						result.URL = v
					}
					if v, ok := rm["content"].(string); ok {
						result.Content = v
					}
					if v, ok := rm["score"].(float64); ok {
						result.Score = v
					}
					if v, ok := rm["raw_content"].(string); ok {
						result.RawContent = &v
					}
					out = append(out, result)
				}
			}
			return out
		}
	}
	return []models.SearchResult{}
}

func getImages(m map[string]interface{}) []models.ImageResult {
	if val, ok := m["images"]; ok {
		if images, ok := val.([]interface{}); ok {
			out := make([]models.ImageResult, 0)
			for _, img := range images {
				if imgm, ok := img.(map[string]interface{}); ok {
					image := models.ImageResult{}
					if v, ok := imgm["url"].(string); ok {
						image.URL = v
					}
					if v, ok := imgm["description"].(string); ok {
						image.Description = v
					}
					out = append(out, image)
				}
			}
			return out
		}
	}
	return []models.ImageResult{}
}
