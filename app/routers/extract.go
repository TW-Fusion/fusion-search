package routers

import (
	"context"
	"net/http"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/TW-Fusion/fusion-search/app/middleware"
	"github.com/TW-Fusion/fusion-search/app/models"
	"github.com/TW-Fusion/fusion-search/app/services"
	"github.com/gin-gonic/gin"
)

// ExtractRouter sets up extract routes
func ExtractRouter(
	r *gin.Engine,
	cfg *config.AppConfig,
	cache *services.CacheService,
	extractor *services.ContentExtractor,
	rateLimiter *middleware.RateLimiter,
) {
	router := r.Group("/extract")
	router.Use(rateLimiter.Limit(cfg.RateLimit.ExtractRate))
	router.POST("", ExtractHandler(cfg, cache, extractor))
}

// ExtractHandler godoc
// @Summary Extract clean content from one or more URLs
// @Description Extract and clean content from web pages, returning in markdown or text format. Supports batch extraction with caching for better performance.
// @Tags Extraction
// @Accept json
// @Produce json
// @Param request body models.ExtractRequest true "Extract request parameters"
// @Success 200 {object} models.ExtractResponse "Successful extraction response"
// @Failure 400 {object} map[string]string "Invalid request parameters"
// @Router /extract [post]
func ExtractHandler(
	cfg *config.AppConfig,
	cache *services.CacheService,
	extractor *services.ContentExtractor,
) gin.HandlerFunc {
	logger := logging.GetLogger()

	return func(c *gin.Context) {
		var req models.ExtractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Format == "" {
			req.Format = models.ContentFormatMarkdown
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(cfg.Resilience.RequestTimeout)*time.Second)
		defer cancel()

		start := time.Now()

		results := make([]models.ExtractResult, 0)
		failed := make([]models.FailedResult, 0)

		// Batch cache lookup
		cachedMap, err := cache.GetExtractBatch(req.URLs)
		if err != nil {
			logger.Errorw("cache_lookup_failed", "error", err)
			cachedMap = make(map[string]*string)
			for _, url := range req.URLs {
				cachedMap[url] = nil
			}
		}

		urlsToFetch := make([]string, 0)
		for url, content := range cachedMap {
			if content != nil {
				results = append(results, models.ExtractResult{
					URL:        url,
					RawContent: *content,
				})
			} else {
				urlsToFetch = append(urlsToFetch, url)
			}
		}

		// Fetch uncached URLs
		if len(urlsToFetch) > 0 {
			extractions := extractor.ExtractURLs(ctx, urlsToFetch, string(req.Format))
			pairs := [][2]string{}
			for _, extraction := range extractions {
				if extraction.Success() {
					results = append(results, models.ExtractResult{
						URL:        extraction.URL,
						RawContent: *extraction.Content,
					})
					pairs = append(pairs, [2]string{extraction.URL, *extraction.Content})
				} else {
					errMsg := "Unknown error"
					if extraction.Error != nil {
						errMsg = *extraction.Error
					}
					failed = append(failed, models.FailedResult{
						URL:   extraction.URL,
						Error: errMsg,
					})
				}
			}
			if len(pairs) > 0 {
				cache.SetExtractBatch(pairs)
			}
		}

		elapsed := time.Since(start).Seconds()
		c.JSON(http.StatusOK, models.ExtractResponse{
			Results:       results,
			FailedResults: failed,
			ResponseTime:  elapsed,
		})
	}
}
