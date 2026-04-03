package routers

import (
	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/middleware"
	"github.com/TW-Fusion/fusion-search/app/services"
	"github.com/gin-gonic/gin"
)

// Setup registers all routes with the Gin engine
func Setup(
	r *gin.Engine,
	cfg *config.AppConfig,
	cacheService *services.CacheService,
	searchBackend services.SearchBackend,
	contentExtractor *services.ContentExtractor,
	llmService *services.LLMService,
	rateLimiter *middleware.RateLimiter,
) {
	// API routes
	SearchRouter(r, cfg, cacheService, searchBackend, contentExtractor, llmService, rateLimiter)
	ExtractRouter(r, cfg, cacheService, contentExtractor, rateLimiter)
	SearchStreamRouter(r, cfg, searchBackend, contentExtractor, llmService, rateLimiter)

	// Utility routes
	HealthRouter(r)
	ToolSchemaRouter(r)
}
