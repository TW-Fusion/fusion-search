package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/TW-Fusion/fusion-search/app/middleware"
	"github.com/TW-Fusion/fusion-search/app/routers"
	"github.com/TW-Fusion/fusion-search/app/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// @title FusionSearch API
// @version 1.0
// @description API for web search, content extraction, and AI-powered answers
// @host localhost:9000
// @BasePath /
// @schemes http https

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("")
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Initialize logging
	if err := logging.InitLogger(cfg.Logging.Format, cfg.Logging.Level); err != nil {
		panic(fmt.Sprintf("Failed to initialize logging: %v", err))
	}

	logger := logging.GetLogger()
	logger.Info("starting", "service", "FusionSearch")

	// Create HTTP clients
	searchHTTPClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	extractHTTPClient := &http.Client{
		Timeout: time.Duration(cfg.Extraction.Timeout+5) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// Initialize services
	cacheService := services.NewCacheService(cfg)
	if err := cacheService.Connect(); err != nil {
		logger.Errorw("cache_connection_failed", "error", err)
	}

	searchBackend := services.CreateSearchBackend(cfg, searchHTTPClient)

	contentExtractor := services.NewContentExtractor(cfg, extractHTTPClient)

	llmService := services.NewLLMService(cfg)
	llmService.Initialize()

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(cfg)
	defer rateLimiter.Close()

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Middleware (order matters - last added is first executed)
	r.Use(gin.Recovery())
	r.Use(middleware.TimingMiddleware())
	r.Use(middleware.RequestIDMiddleware())

	// CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = cfg.Cors.AllowOrigins
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	r.Use(cors.New(corsConfig))

	// Auth middleware
	r.Use(middleware.AuthMiddleware(cfg))

	// Setup routes
	routers.Setup(r, cfg, cacheService, searchBackend, contentExtractor, llmService, rateLimiter)

	// Swagger UI (development only)
	setupSwagger(r)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("ready",
		"service", "FusionSearch",
		"backend", cfg.Search.Backend,
		"cache", func() string {
			if cacheService.IsEnabled() {
				return "enabled"
			}
			return "disabled"
		}(),
		"auth", func() string {
			if cfg.Auth.Enabled {
				return "enabled"
			}
			return "disabled"
		}(),
		"rate_limit", func() string {
			if cfg.RateLimit.Enabled {
				return "enabled"
			}
			return "disabled"
		}(),
		"rerank", func() string {
			if cfg.Rerank.Enabled {
				return "enabled"
			}
			return "disabled"
		}(),
		"llm", func() string {
			if cfg.LLM.Enabled {
				return "enabled"
			}
			return "disabled"
		}(),
		"fallback", func() string {
			if cfg.Resilience.BackendFallback {
				return "enabled"
			}
			return "disabled"
		}(),
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalw("server_failed", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting_down", "service", "FusionSearch")

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorw("server_shutdown_failed", "error", err)
	}

	// Cleanup services
	cacheService.Close()
	llmService.Close()

	logger.Info("shutdown", "service", "FusionSearch")
}
